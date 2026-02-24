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
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleScreenshot(w http.ResponseWriter, r *http.Request) {
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
		bridge.DisableAnimationsOnce(tCtx)
	}

	var buf []byte
	quality := 80
	if q := r.URL.Query().Get("quality"); q != "" {
		if qn, err := strconv.Atoi(q); err == nil {
			quality = qn
		}
	}

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(quality)).
				Do(ctx)
			return err
		}),
	); err != nil {
		web.Error(w, 500, fmt.Errorf("screenshot: %w", err))
		return
	}

	if output == "file" {
		screenshotDir := filepath.Join(h.Config.StateDir, "screenshots")
		if err := os.MkdirAll(screenshotDir, 0755); err != nil {
			web.Error(w, 500, fmt.Errorf("create screenshot dir: %w", err))
			return
		}

		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("screenshot-%s.jpg", timestamp)
		filePath := filepath.Join(screenshotDir, filename)

		if err := os.WriteFile(filePath, buf, 0644); err != nil {
			web.Error(w, 500, fmt.Errorf("write screenshot: %w", err))
			return
		}

		web.JSON(w, 200, map[string]any{
			"path":      filePath,
			"size":      len(buf),
			"format":    "jpeg",
			"timestamp": timestamp,
		})
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "image/jpeg")
		if _, err := w.Write(buf); err != nil {
			slog.Error("screenshot write", "err", err)
		}
		return
	}

	web.JSON(w, 200, map[string]any{
		"format": "jpeg",
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

func (h *Handlers) HandlePDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output")

	ctx, _, err := h.Bridge.TabContext(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	landscape := r.URL.Query().Get("landscape") == "true"
	scale := 1.0
	if s := r.URL.Query().Get("scale"); s != "" {
		if sn, err := strconv.ParseFloat(s, 64); err == nil && sn > 0 {
			scale = sn
		}
	}

	var buf []byte
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			p := page.PrintToPDF().
				WithPrintBackground(true).
				WithScale(scale).
				WithLandscape(landscape)
			buf, _, err = p.Do(ctx)
			return err
		}),
	); err != nil {
		web.Error(w, 500, fmt.Errorf("pdf: %w", err))
		return
	}

	if output == "file" {
		savePath := r.URL.Query().Get("path")
		if savePath == "" {
			pdfDir := filepath.Join(h.Config.StateDir, "pdfs")
			if err := os.MkdirAll(pdfDir, 0755); err != nil {
				web.Error(w, 500, fmt.Errorf("create pdf dir: %w", err))
				return
			}
			timestamp := time.Now().Format("20060102-150405")
			savePath = filepath.Join(pdfDir, fmt.Sprintf("page-%s.pdf", timestamp))
		}

		if err := os.WriteFile(savePath, buf, 0644); err != nil {
			web.Error(w, 500, fmt.Errorf("write pdf: %w", err))
			return
		}

		web.JSON(w, 200, map[string]any{
			"path": savePath,
			"size": len(buf),
		})
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "application/pdf")
		if _, err := w.Write(buf); err != nil {
			slog.Error("pdf write", "err", err)
		}
		return
	}

	web.JSON(w, 200, map[string]any{
		"format": "pdf",
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

func (h *Handlers) HandleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	mode := r.URL.Query().Get("mode")

	ctx, _, err := h.Bridge.TabContext(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	var text string
	if mode == "raw" {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(`document.body.innerText`, &text),
		); err != nil {
			web.Error(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	} else {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(assets.ReadabilityJS, &text),
		); err != nil {
			web.Error(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	}

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	web.JSON(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"text":  text,
	})
}
