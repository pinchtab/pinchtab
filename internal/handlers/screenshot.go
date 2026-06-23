package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type screenshotRequest struct {
	tabID          string
	output         string
	selector       string
	noAnimations   bool
	annotate       bool
	beyondViewport bool
	quality        int
	format         string // "jpeg" | "png"
	scale          float64
	raw            bool
}

func parseScreenshotRequest(r *http.Request) screenshotRequest {
	q := r.URL.Query()
	req := screenshotRequest{
		tabID:        q.Get("tabId"),
		output:       q.Get("output"),
		selector:     q.Get("selector"),
		noAnimations: q.Get("noAnimations") == "true",
		annotate:     q.Get("annotate") == "true" || q.Get("annotate") == "1",
		quality:      80,
		format:       "jpeg",
		scale:        1.0,
		raw:          q.Get("raw") == "true",
	}

	// beyondViewport captures the full scrollable document. selector wins
	// silently when both are set — selector already clips to an element, and
	// stacking a doc-sized clip on top would be meaningless.
	req.beyondViewport = q.Get("beyondViewport") == "true" || q.Get("beyondViewport") == "1"
	if req.selector != "" {
		req.beyondViewport = false
	}

	// Invalid quality/scale silently fall back to defaults (no error).
	if v := q.Get("quality"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.quality = n
		}
	}
	if q.Get("format") == "png" {
		req.format = "png"
	}
	if v := q.Get("scale"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			req.scale = bridge.ClampScale(f)
		}
	}
	return req
}

// @Endpoint GET /screenshot
func (h *Handlers) HandleScreenshot(w http.ResponseWriter, r *http.Request) {
	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}

	req := parseScreenshotRequest(r)

	resolvedTabID, tCtx, cancel, ok := h.resolveBinaryReadContext(w, r, req.tabID, h.Config.ActionTimeout)
	if !ok {
		return
	}
	defer cancel()

	if req.noAnimations && !h.Config.NoAnimations {
		if err := bridge.DisableAnimationsOnce(tCtx); err != nil {
			httpx.Error(w, 500, fmt.Errorf("disable animations: %w", err))
			return
		}
	}

	if req.annotate {
		h.serveAnnotatedScreenshot(w, tCtx, resolvedTabID, req)
		return
	}

	clip, clipErr := h.resolveScreenshotClip(tCtx, resolvedTabID, req.selector)
	if clipErr != nil {
		httpx.Error(w, clipErr.status, clipErr.err)
		return
	}

	// Route through the shared bridge.CaptureScreenshot engine: it carries the
	// scale/beyondViewport clip synthesis and the provider-runtime fixes
	// (BringToFront + WithFromSurface(false)) so capture works on chrome and
	// cloak alike; tCtx already targets the active provider's CDP session.
	cdpFormat := bridge.ScreenshotFormatJpeg
	if req.format == "png" {
		cdpFormat = bridge.ScreenshotFormatPng
	}
	buf, captureErr := bridge.CaptureScreenshot(tCtx, bridge.ScreenshotOpts{
		Format:         cdpFormat,
		Quality:        req.quality,
		Clip:           clip,
		BeyondViewport: req.beyondViewport,
		Scale:          req.scale,
	})
	if captureErr != nil {
		httpx.Error(w, 500, fmt.Errorf("screenshot: %w", captureErr))
		return
	}

	if req.output == "file" {
		h.writeScreenshotFile(w, buf, req.format)
		return
	}

	if req.raw {
		writeRawImage(w, buf, imageContentType(req.format), "screenshot write")
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"format": req.format,
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

// statusError pairs an HTTP status with the error to serialize.
type statusError struct {
	status int
	err    error
}

// resolveScreenshotClip resolves the selector (if any) to a CDP screenshot clip.
// Returns nil clip when no selector is set (full-page / beyondViewport capture).
func (h *Handlers) resolveScreenshotClip(ctx context.Context, tabID, selector string) (*bridge.ScreenshotClip, *statusError) {
	if selector == "" {
		return nil, nil
	}
	nodeID, err := h.resolveSelectorNodeID(ctx, tabID, selector)
	if err != nil {
		return nil, &statusError{400, frameScopedSelectorError("selector", err)}
	}
	clip, err := bridge.ScreenshotClipForNode(ctx, nodeID)
	if err != nil {
		return nil, &statusError{500, fmt.Errorf("selector screenshot: %w", err)}
	}
	return clip, nil
}

func (h *Handlers) serveAnnotatedScreenshot(w http.ResponseWriter, ctx context.Context, tabID string, req screenshotRequest) {
	img, items, outFormat, err := h.captureAnnotatedScreenshot(ctx, tabID, req.selector, req.format, req.quality, req.beyondViewport)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("annotate: %w", err))
		return
	}
	if req.raw {
		writeRawImage(w, img, imageContentType(outFormat), "annotated screenshot write")
		return
	}
	httpx.JSON(w, 200, map[string]any{
		"format":      outFormat,
		"base64":      base64.StdEncoding.EncodeToString(img),
		"annotations": items,
	})
}

func (h *Handlers) writeScreenshotFile(w http.ResponseWriter, buf []byte, format string) {
	filePath, timestamp, err := saveBinaryToStateDir(h.Config.StateDir, "screenshots", "screenshot", imageExt(format), buf)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("write screenshot: %w", err))
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"path":      filePath,
		"size":      len(buf),
		"format":    format,
		"timestamp": timestamp,
	})
}

// @Endpoint GET /tabs/{id}/screenshot
func (h *Handlers) HandleTabScreenshot(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleScreenshot)
}
