package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleText extracts readable text from the current tab.
//
// @Endpoint GET /text
func (h *Handlers) HandleText(w http.ResponseWriter, r *http.Request) {
	// --- Lite engine fast path ---
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "text", tabID)
	if h.useLite(engine.CapText, "") {
		h.recordEngine(r, "lite")
		text, err := h.Router.Lite().Text(r.Context(), tabID)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("lite text: %w", err))
			return
		}
		w.Header().Set("X-Engine", "lite")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(text))
		return
	}

	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	mode := r.URL.Query().Get("mode")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	maxChars := -1
	if v := r.URL.Query().Get("maxChars"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxChars = n
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	var text string
	if mode == "raw" {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(`document.body.innerText`, &text),
		); err != nil {
			httpx.Error(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	} else {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(assets.ReadabilityJS, &text),
		); err != nil {
			httpx.Error(w, 500, fmt.Errorf("text extract: %w", err))
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
	h.recordResolvedURL(r, url)

	// IDPI: scan extracted text for injection patterns before it reaches the caller.
	idpiResult := h.IDPIGuard.ScanContent(text)
	if idpiResult.Blocked {
		httpx.Error(w, http.StatusForbidden,
			fmt.Errorf("content blocked by IDPI scanner: %s", idpiResult.Reason))
		return
	}
	if idpiResult.Threat {
		w.Header().Set("X-IDPI-Warning", idpiResult.Reason)
		if idpiResult.Pattern != "" {
			w.Header().Set("X-IDPI-Pattern", idpiResult.Pattern)
		}
	}

	// IDPI: wrap plain-text content in <untrusted_web_content> delimiters so
	// downstream LLMs treat it as data, not instructions.
	if h.Config.IDPI.Enabled && h.Config.IDPI.WrapContent {
		text = h.IDPIGuard.WrapContent(text, url)
	}

	if format == "text" || format == "plain" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(text))
		return
	}

	resp := map[string]any{
		"url":       url,
		"title":     title,
		"text":      text,
		"truncated": truncated,
	}
	if idpiResult.Threat {
		resp["idpiWarning"] = idpiResult.Reason
	}
	httpx.JSON(w, 200, resp)
}

// HandleTabText extracts text for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/text
func (h *Handlers) HandleTabText(w http.ResponseWriter, r *http.Request) {
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

	h.HandleText(w, req)
}
