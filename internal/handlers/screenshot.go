package handlers

import (
	"context"
	"encoding/base64"
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
	"github.com/pinchtab/pinchtab/internal/web"
)

// HandleScreenshot captures a screenshot of the current tab.
//
// @Endpoint GET /screenshot
func (h *Handlers) HandleScreenshot(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output")
	reqNoAnim := r.URL.Query().Get("noAnimations") == "true"

	ctx, _, err := h.Bridge.TabContext(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if reqNoAnim && !h.Config.NoAnimations {
		if err := bridge.DisableAnimationsOnce(tCtx); err != nil {
			web.Error(w, 500, fmt.Errorf("disable animations: %w", err))
			return
		}
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

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			shot := page.CaptureScreenshot().WithFormat(format)
			if format == page.CaptureScreenshotFormatJpeg {
				shot = shot.WithQuality(int64(quality))
			}
			buf, err = shot.Do(ctx)
			return err
		}),
	); err != nil {
		web.Error(w, 500, fmt.Errorf("screenshot: %w", err))
		return
	}

	if output == "file" {
		screenshotDir := filepath.Join(h.Config.StateDir, "screenshots")
		if err := os.MkdirAll(screenshotDir, 0750); err != nil {
			web.Error(w, 500, fmt.Errorf("create screenshot dir: %w", err))
			return
		}

		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("screenshot-%s%s", timestamp, ext)
		filePath := filepath.Join(screenshotDir, filename)

		if err := os.WriteFile(filePath, buf, 0600); err != nil {
			web.Error(w, 500, fmt.Errorf("write screenshot: %w", err))
			return
		}

		web.JSON(w, 200, map[string]any{
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

	web.JSON(w, 200, map[string]any{
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
		web.Error(w, 400, fmt.Errorf("tab id required"))
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
