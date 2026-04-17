package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
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
		result, err := h.Router.Lite().Text(r.Context(), tabID)
		if err != nil {
			if engine.IsIDPIBlocked(err) {
				httpx.Error(w, http.StatusForbidden, err)
			} else {
				httpx.Error(w, 500, fmt.Errorf("lite text: %w", err))
			}
			return
		}
		w.Header().Set("X-Engine", "lite")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(result.Text))
		return
	}

	h.recordEngine(r, "chrome")
	w.Header().Set("X-Engine", "chrome")

	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
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

	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
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

	// Resolve the target frame. Explicit ?frameId= wins; otherwise fall back
	// to the currently-scoped frame on this tab (as set by /frame). Empty
	// means "top-level document" and preserves the prior behaviour.
	targetFrameID := r.URL.Query().Get("frameId")
	if targetFrameID == "" {
		if scope, ok := h.currentFrameScope(resolvedTabID); ok {
			targetFrameID = scope.FrameID
		}
	}

	script := `document.body.innerText`
	if mode != "raw" {
		script = assets.ReadabilityJS
	}

	var text string
	if targetFrameID == "" {
		// Top-frame path — keep the ergonomic chromedp.Evaluate helper.
		if err := chromedp.Run(tCtx, chromedp.Evaluate(script, &text)); err != nil {
			httpx.Error(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	} else {
		// Frame-scoped path — evaluate in the frame's isolated world so the
		// expression sees the iframe's `document`, not the parent's.
		execID, err := bridge.FrameExecutionContextID(tCtx, targetFrameID)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("resolve frame context: %w", err))
			return
		}
		var raw json.RawMessage
		err = chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
				"expression":    script,
				"returnByValue": true,
				"contextId":     execID,
			}, &raw)
		}))
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("text extract (frame %s): %w", targetFrameID, err))
			return
		}
		var er struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
			ExceptionDetails *struct {
				Text string `json:"text"`
			} `json:"exceptionDetails,omitempty"`
		}
		if err := json.Unmarshal(raw, &er); err != nil {
			httpx.Error(w, 500, fmt.Errorf("text extract parse: %w", err))
			return
		}
		if er.ExceptionDetails != nil && er.ExceptionDetails.Text != "" {
			httpx.Error(w, 500, fmt.Errorf("text extract (frame %s): %s", targetFrameID, er.ExceptionDetails.Text))
			return
		}
		text = er.Result.Value
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
