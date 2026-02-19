package main

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
)

func (b *Bridge) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output")
	reqNoAnim := r.URL.Query().Get("noAnimations") == "true"

	ctx, _, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, cfg.ActionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	if reqNoAnim && !cfg.NoAnimations {
		disableAnimationsOnce(tCtx)
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
		jsonErr(w, 500, fmt.Errorf("screenshot: %w", err))
		return
	}

	if output == "file" {
		screenshotDir := filepath.Join(cfg.StateDir, "screenshots")
		if err := os.MkdirAll(screenshotDir, 0755); err != nil {
			jsonErr(w, 500, fmt.Errorf("create screenshot dir: %w", err))
			return
		}

		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("screenshot-%s.jpg", timestamp)
		filePath := filepath.Join(screenshotDir, filename)

		if err := os.WriteFile(filePath, buf, 0644); err != nil {
			jsonErr(w, 500, fmt.Errorf("write screenshot: %w", err))
			return
		}

		jsonResp(w, 200, map[string]any{
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

	jsonResp(w, 200, map[string]any{
		"format": "jpeg",
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

func (b *Bridge) handleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	mode := r.URL.Query().Get("mode")

	ctx, _, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, cfg.ActionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var text string
	if mode == "raw" {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(`document.body.innerText`, &text),
		); err != nil {
			jsonErr(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	} else {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(readabilityJS, &text),
		); err != nil {
			jsonErr(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	}

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"text":  text,
	})
}
