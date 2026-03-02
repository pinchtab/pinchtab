package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/web"
)

// HandleText extracts readable text from the current tab.
//
// @Endpoint GET /text
func (h *Handlers) HandleText(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	mode := r.URL.Query().Get("mode")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	maxChars := -1
	if v := r.URL.Query().Get("maxChars"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxChars = n
		}
	}

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

	truncated := false
	if maxChars > -1 && len(text) > maxChars {
		text = text[:maxChars]
		truncated = true
	}

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	if format == "text" || format == "plain" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(text))
		return
	}

	web.JSON(w, 200, map[string]any{
		"url":       url,
		"title":     title,
		"text":      text,
		"truncated": truncated,
	})
}

// HandleTabText extracts text for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/text
func (h *Handlers) HandleTabText(w http.ResponseWriter, r *http.Request) {
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

	h.HandleText(w, req)
}
