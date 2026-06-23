package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleCapture returns a screenshot and an accessibility snapshot from the
// same DOM epoch in one HTTP call.
//
// @Endpoint GET /capture
// @Description Paired screenshot + accessibility snapshot. Returns both
//
//	artefacts plus a frame/loader parity check (pairing.navigated) and an
//	opaque domEpoch handshake token cached on the tab's RefCache.
//
// @Param tabId string query Tab ID (optional, defaults to current)
// @Param selector string query Optional scope (clips screenshot and filters snapshot subtree)
// @Param filter string query Snapshot filter: "interactive" or "all" (default "interactive")
// @Param depth int query Snapshot max depth (default -1 for full)
// @Param format string query Image format: "jpeg" or "png" (default "jpeg")
// @Param quality int query JPEG quality 1-100 (default 80)
// @Param output string query "file" writes the image to disk; "inline" returns base64; default "file"
// @Param requirePair bool query If true, return 409 when navigation is observed during capture
// @Param noAnimations bool query Inject reduce-motion CSS for the capture window
//
// @Response 200 application/json Paired result (see response shape below)
// @Response 409 application/json Pair was broken (only when requirePair=true)
// @Response 404 application/json Tab not found
// @Response 500 application/json Internal error
func (h *Handlers) HandleCapture(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tabID := q.Get("tabId")

	// /capture pairs a screenshot with an accessibility snapshot from the same
	// DOM epoch — a visual operation. The static (ghost-chrome) runtime cannot
	// paint, so it skips ShapeVisual and we fall back to Chrome.
	effectiveCfg, _, ok := h.resolveReadRouting(w, r, tabID, "capture", browsers.ShapeVisual)
	if !ok {
		return
	}

	if !h.ensureBrowserOrRespond(w, effectiveCfg) {
		return
	}

	selector := q.Get("selector")
	filter := q.Get("filter")
	if filter == "" {
		filter = bridge.FilterInteractive
	}
	output := q.Get("output")
	if output == "" {
		output = "file"
	}
	requirePair := q.Get("requirePair") == "true" || q.Get("requirePair") == "1"
	reqNoAnim := q.Get("noAnimations") == "true"
	beyondViewport := q.Get("beyondViewport") == "true" || q.Get("beyondViewport") == "1"

	// Clamp so ?scale=10000 can't ask CDP for a gigapixel image.
	scale := 1.0
	if s := q.Get("scale"); s != "" {
		if sf, err := strconv.ParseFloat(s, 64); err == nil {
			scale = bridge.ClampScale(sf)
		}
	}

	// Default withBounds=true. Bounding boxes are the piece that lets vision
	// agents overlay refs on pixels — the whole point of /capture. Callers
	// who want to skip the per-node box-model round trips can pass
	// withBounds=false.
	withBounds := true
	if v := q.Get("withBounds"); v == "false" || v == "0" {
		withBounds = false
	}

	// Default to wait=stable: vision-grounded callers are the primary user
	// and they want a quiesced page. wait=load polls document.readyState;
	// wait=none skips the wait entirely.
	wait := q.Get("wait")
	if wait == "" {
		wait = bridge.WaitStable
	}

	depth := -1
	if d := q.Get("depth"); d != "" {
		if dn, err := strconv.Atoi(d); err == nil {
			depth = dn
		}
	}
	quality := 80
	if qv := q.Get("quality"); qv != "" {
		if qn, err := strconv.Atoi(qv); err == nil {
			quality = qn
		}
	}

	format := bridge.ScreenshotFormatJpeg
	ext := ".jpg"
	if q.Get("format") == "png" {
		format = bridge.ScreenshotFormatPng
		ext = ".png"
	}

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, tabID, effectiveCfg.ActionTimeout)
	if !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)
	defer cancel()

	opts := bridge.CaptureOpts{
		Image: bridge.ScreenshotOpts{
			Format:         format,
			Quality:        quality,
			BeyondViewport: beyondViewport,
			Scale:          scale,
		},
		Filter:            filter,
		MaxDepth:          depth,
		ScopeFrameID:      h.selectorFrameID(resolvedTabID),
		DisableAnimations: reqNoAnim && !h.Config.NoAnimations,
		Wait:              wait,
		WithBounds:        withBounds,
	}

	// Selector scoping: resolve once, use the same backendNodeId for both the
	// screenshot clip and the snapshot subtree filter so they describe the
	// same element. A selector clip silently wins over beyondViewport — the
	// same rule HandleScreenshot enforces.
	if selector != "" {
		opts.Image.BeyondViewport = false
		nodeID, sErr := h.resolveSelectorNodeID(tCtx, resolvedTabID, selector)
		if sErr != nil {
			httpx.Error(w, 400, frameScopedSelectorError("selector", sErr))
			return
		}
		opts.ScopeBackendNodeID = nodeID
		clip, cErr := bridge.ScreenshotClipForNode(tCtx, nodeID)
		if cErr != nil {
			httpx.Error(w, 500, fmt.Errorf("selector clip: %w", cErr))
			return
		}
		opts.Image.Clip = clip
	}

	result, err := bridge.PairedCapture(tCtx, opts)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("capture: %w", err))
		return
	}
	h.recordResolvedURL(r, result.URL)

	if requirePair && result.Navigated {
		httpx.Error(w, http.StatusConflict,
			fmt.Errorf("pairing broken: navigation observed during capture window"))
		return
	}

	// IDPI scan: same contract as HandleSnapshot. Run before any file write
	// so a blocked capture doesn't leave an orphan image on disk.
	idpiResult := h.scanSnapshotIDPI(w, result.Nodes)
	if idpiResult.Blocked {
		return
	}

	// Persist the snapshot half to the ref cache with the minted epoch so that
	// follow-up `/click eN` etc. can later opt into an epoch handshake.
	h.Bridge.SetRefCache(resolvedTabID, &bridge.RefCache{
		Refs:     result.Refs,
		Targets:  bridge.RefTargetsFromNodes(result.Nodes),
		Nodes:    result.Nodes,
		DomEpoch: result.DomEpoch,
	})

	imageInfo := map[string]any{
		"format":           result.ImageFormat,
		"bytes":            len(result.ImageBytes),
		"coordinateSpace":  result.CoordinateSpace,
		"devicePixelRatio": result.Viewport.DevicePixelRatio,
		"viewport": map[string]any{
			"w":       result.Viewport.Width,
			"h":       result.Viewport.Height,
			"scrollX": result.Viewport.ScrollX,
			"scrollY": result.Viewport.ScrollY,
		},
	}
	if result.Clip != nil {
		imageInfo["clip"] = map[string]any{
			"x": result.Clip.X,
			"y": result.Clip.Y,
			"w": result.Clip.Width,
			"h": result.Clip.Height,
		}
	}

	switch output {
	case "file":
		filePath, _, err := saveBinaryToStateDir(h.Config.StateDir, "captures", "cap", ext, result.ImageBytes)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("write capture: %w", err))
			return
		}
		imageInfo["path"] = filePath
	case "inline":
		imageInfo["base64"] = base64.StdEncoding.EncodeToString(result.ImageBytes)
	case "raw":
		// Raw output skips the JSON envelope and returns image bytes only.
		// The snapshot half is dropped; this mode is for clients that already
		// have a separate /snapshot call. It exists mostly as a debug aid.
		writeRawImage(w, result.ImageBytes, imageContentType(result.ImageFormat), "capture raw write")
		return
	default:
		httpx.Error(w, 400, fmt.Errorf("unknown output %q (expected file|inline|raw)", output))
		return
	}

	resp := map[string]any{
		"status":     "ok",
		"tabId":      resolvedTabID,
		"url":        result.URL,
		"title":      result.Title,
		"capturedAt": result.CapturedAt.UTC().Format(time.RFC3339Nano),
		"epoch": map[string]any{
			"frameId":  result.FrameID,
			"loaderId": result.LoaderID,
			"domEpoch": result.DomEpoch,
		},
		"pairing": map[string]any{
			"navigated":         result.Navigated,
			"captureDurationMs": result.DurationMs,
		},
		"image": imageInfo,
		"snapshot": map[string]any{
			"filter":    result.Filter,
			"nodeCount": len(result.Nodes),
			"nodes":     result.Nodes,
		},
	}
	if idpiResult.Threat {
		resp["idpiWarning"] = idpiResult.Reason
	}
	if idpiResult.WrapContent {
		resp["untrustedContent"] = true
		resp["idpiNotice"] = idpiNoticeText
	}
	httpx.JSON(w, 200, resp)
}

// @Endpoint GET /tabs/{id}/capture
func (h *Handlers) HandleTabCapture(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleCapture)
}
