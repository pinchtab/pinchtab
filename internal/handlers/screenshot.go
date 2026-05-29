package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleScreenshot captures a screenshot of the current tab.
//
// @Endpoint GET /screenshot
func (h *Handlers) HandleScreenshot(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output")
	selector := r.URL.Query().Get("selector")
	reqNoAnim := r.URL.Query().Get("noAnimations") == "true"
	annotate := r.URL.Query().Get("annotate") == "true" || r.URL.Query().Get("annotate") == "1"
	// beyondViewport captures the full scrollable document. selector wins
	// silently when both are set — selector already clips to an element, and
	// stacking a doc-sized clip on top would be meaningless.
	beyondViewport := r.URL.Query().Get("beyondViewport") == "true" || r.URL.Query().Get("beyondViewport") == "1"
	if selector != "" {
		beyondViewport = false
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	if reqNoAnim && !h.Config.NoAnimations {
		if err := bridge.DisableAnimationsOnce(tCtx); err != nil {
			httpx.Error(w, 500, fmt.Errorf("disable animations: %w", err))
			return
		}
	}

	if annotate {
		quality := 80
		if q := r.URL.Query().Get("quality"); q != "" {
			if qn, err := strconv.Atoi(q); err == nil {
				quality = qn
			}
		}
		fmtStr := "jpeg"
		if r.URL.Query().Get("format") == "png" {
			fmtStr = "png"
		}
		img, items, outFormat, err := h.captureAnnotatedScreenshot(tCtx, resolvedTabID, selector, fmtStr, quality, beyondViewport)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("annotate: %w", err))
			return
		}
		contentType := "image/jpeg"
		if outFormat == "png" {
			contentType = "image/png"
		}
		if r.URL.Query().Get("raw") == "true" {
			w.Header().Set("Content-Type", contentType)
			if _, err := w.Write(img); err != nil {
				slog.Error("annotated screenshot write", "err", err)
			}
			return
		}
		httpx.JSON(w, 200, map[string]any{
			"format":      outFormat,
			"base64":      base64.StdEncoding.EncodeToString(img),
			"annotations": items,
		})
		return
	}

	var clip *page.Viewport
	if selector != "" {
		nodeID, err := h.resolveSelectorNodeID(tCtx, resolvedTabID, selector)
		if err != nil {
			httpx.Error(w, 400, frameScopedSelectorError("selector", err))
			return
		}
		clip, err = screenshotClipForNode(tCtx, nodeID)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("selector screenshot: %w", err))
			return
		}
	} else if beyondViewport {
		docW, docH, err := documentSize(tCtx)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("document size: %w", err))
			return
		}
		clip = &page.Viewport{X: 0, Y: 0, Width: docW, Height: docH, Scale: 1}
	}

	var buf []byte
	quality := 80
	if q := r.URL.Query().Get("quality"); q != "" {
		if qn, err := strconv.Atoi(q); err == nil {
			quality = qn
		}
	}

	format := page.CaptureScreenshotFormatJpeg
	contentType := "image/jpeg"
	ext := ".jpg"

	if r.URL.Query().Get("format") == "png" {
		format = page.CaptureScreenshotFormatPng
		contentType = "image/png"
		ext = ".png"
	}

	scale := 1.0
	if s := r.URL.Query().Get("scale"); s != "" {
		if sf, err := strconv.ParseFloat(s, 64); err == nil {
			scale = bridge.ClampScale(sf)
		}
	}
	buf, err = bridge.CaptureScreenshot(tCtx, bridge.ScreenshotOpts{
		Format:         format,
		Quality:        quality,
		Clip:           clip,
		BeyondViewport: beyondViewport,
		Scale:          scale,
	})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("screenshot: %w", err))
		return
	}

	if output == "file" {
		screenshotDir := filepath.Join(h.Config.StateDir, "screenshots")
		if err := os.MkdirAll(screenshotDir, 0750); err != nil {
			httpx.Error(w, 500, fmt.Errorf("create screenshot dir: %w", err))
			return
		}

		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("screenshot-%s%s", timestamp, ext)
		filePath := filepath.Join(screenshotDir, filename)

		if err := os.WriteFile(filePath, buf, 0600); err != nil {
			httpx.Error(w, 500, fmt.Errorf("write screenshot: %w", err))
			return
		}

		httpx.JSON(w, 200, map[string]any{
			"path":      filePath,
			"size":      len(buf),
			"format":    string(format),
			"timestamp": timestamp,
		})
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", contentType)
		if _, err := w.Write(buf); err != nil {
			slog.Error("screenshot write", "err", err)
		}
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"format": string(format),
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

// HandleTabScreenshot returns screenshot bytes for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/screenshot
func (h *Handlers) HandleTabScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.HandleScreenshot(w, req)
}

func screenshotClipForNode(ctx context.Context, nodeID int64) (*page.Viewport, error) {
	// Bring target element into view before computing clip coordinates.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{
			"backendNodeId": nodeID,
		}, nil)
	})); err != nil {
		return nil, fmt.Errorf("scroll into view: %w", err)
	}

	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolveResult)
	})); err != nil {
		return nil, fmt.Errorf("resolve node: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return nil, fmt.Errorf("parse resolved node: %w", err)
	}
	if resolved.Object.ObjectID == "" {
		return nil, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	// Translate the element box into top-level page coordinates. captureScreenshot
	// clip coordinates are page-relative, so viewport-relative rects need scroll
	// offsets from the current document and each ancestor frame.
	const boxFn = `function() {
		const rect = this.getBoundingClientRect();
		let x = rect.left + (window.scrollX || window.pageXOffset || 0);
		let y = rect.top + (window.scrollY || window.pageYOffset || 0);
		try {
			let current = window;
			while (current && current.parent && current !== current.parent) {
				const frameEl = current.frameElement;
				if (!frameEl) {
					break;
				}
				const parent = current.parent;
				const frameRect = frameEl.getBoundingClientRect();
				x += frameRect.left + (parent.scrollX || parent.pageXOffset || 0);
				y += frameRect.top + (parent.scrollY || parent.pageYOffset || 0);
				current = parent;
			}
		} catch (e) {
			// Cross-origin ancestors can block frame traversal. Keep the deepest
			// reachable page coordinates in that case.
		}
		return { x, y, width: rect.width, height: rect.height };
	}`

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": boxFn,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return nil, fmt.Errorf("read element box: %w", err)
	}

	var boxCall struct {
		Result struct {
			Value struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &boxCall); err != nil {
		return nil, fmt.Errorf("parse element box: %w", err)
	}

	box := boxCall.Result.Value
	if box.Width <= 0 || box.Height <= 0 {
		return nil, fmt.Errorf("element box is empty (width=%.2f height=%.2f)", box.Width, box.Height)
	}
	return &page.Viewport{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
		Scale:  1,
	}, nil
}
