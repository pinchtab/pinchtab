package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/web"
)

// HandleDownload fetches a URL using the browser's session (cookies, stealth)
// and returns the content. This preserves authentication and fingerprint.
//
// GET /download?url=<url>[&tabId=<id>][&output=file&path=/tmp/file][&raw=true]
func (h *Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	dlURL := r.URL.Query().Get("url")
	if dlURL == "" {
		web.Error(w, 400, fmt.Errorf("url parameter required"))
		return
	}

	output := r.URL.Query().Get("output")
	filePath := r.URL.Query().Get("path")
	raw := r.URL.Query().Get("raw") == "true"

	// Create a temporary tab for the download — avoids navigating the user's tab away.
	browserCtx := h.Bridge.BrowserContext()
	tabCtx, tabCancel := chromedp.NewContext(browserCtx)
	defer tabCancel()

	tCtx, tCancel := context.WithTimeout(tabCtx, 30*time.Second)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	// Enable network tracking to capture response metadata.
	var requestID network.RequestID
	var responseMIME string
	var responseStatus int
	done := make(chan struct{}, 1)

	chromedp.ListenTarget(tCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			if e.Response.URL == dlURL && requestID == "" {
				requestID = e.RequestID
				responseMIME = e.Response.MimeType
				responseStatus = int(e.Response.Status)
			}
		case *network.EventLoadingFinished:
			if e.RequestID == requestID && requestID != "" {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		case *network.EventLoadingFailed:
			if e.RequestID == requestID && requestID != "" {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		}
	})

	if err := chromedp.Run(tCtx, network.Enable()); err != nil {
		web.Error(w, 500, fmt.Errorf("network enable: %w", err))
		return
	}

	// Navigate the temp tab to the URL — uses browser's cookie jar and stealth.
	if err := chromedp.Run(tCtx, chromedp.Navigate(dlURL)); err != nil {
		web.Error(w, 502, fmt.Errorf("navigate to download URL: %w", err))
		return
	}

	// Wait for response.
	select {
	case <-done:
	case <-tCtx.Done():
		web.Error(w, 504, fmt.Errorf("download timed out"))
		return
	}

	if responseStatus >= 400 {
		web.Error(w, 502, fmt.Errorf("remote server returned HTTP %d", responseStatus))
		return
	}

	// Get response body via CDP.
	var body []byte
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			b, err := network.GetResponseBody(requestID).Do(ctx)
			if err != nil {
				return err
			}
			body = b
			return nil
		}),
	); err != nil {
		web.Error(w, 500, fmt.Errorf("get response body: %w", err))
		return
	}

	if responseMIME == "" {
		responseMIME = "application/octet-stream"
	}

	// Write to file.
	if output == "file" {
		if filePath == "" {
			web.Error(w, 400, fmt.Errorf("path required when output=file"))
			return
		}
		safe, pathErr := web.SafePath(h.Config.StateDir, filePath)
		if pathErr != nil {
			web.Error(w, 400, fmt.Errorf("invalid path: %w", pathErr))
			return
		}
		filePath = safe
		if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
			web.Error(w, 500, fmt.Errorf("failed to create directory: %w", err))
			return
		}
		if err := os.WriteFile(filePath, body, 0600); err != nil {
			web.Error(w, 500, fmt.Errorf("failed to write file: %w", err))
			return
		}
		web.JSON(w, 200, map[string]any{
			"status":      "saved",
			"path":        filePath,
			"size":        len(body),
			"contentType": responseMIME,
		})
		return
	}

	// Raw bytes.
	if raw {
		w.Header().Set("Content-Type", responseMIME)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(200)
		_, _ = w.Write(body)
		return
	}

	// Default: base64 JSON response.
	web.JSON(w, 200, map[string]any{
		"data":        base64.StdEncoding.EncodeToString(body),
		"contentType": responseMIME,
		"size":        len(body),
		"url":         dlURL,
	})
}
