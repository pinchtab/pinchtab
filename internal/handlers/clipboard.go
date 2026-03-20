package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type clipboardRequest struct {
	TabID string  `json:"tabId"`
	Text  *string `json:"text"`
}

// evalAwaitPromiseWithGesture wraps chromedp.Evaluate to await a promise result with user gesture.
// The userGesture flag allows clipboard access without explicit permission grants.
func evalAwaitPromiseWithGesture(expression string, res any) chromedp.Action {
	return chromedp.Evaluate(expression, res, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true).WithUserGesture(true)
	})
}

// resolveClipboardTab resolves the tab context for clipboard operations.
func (h *Handlers) resolveClipboardTab(r *http.Request, bodyTabID string) (context.Context, string, error) {
	tabID := strings.TrimSpace(r.URL.Query().Get("tabId"))
	if tabID == "" {
		tabID = strings.TrimSpace(bodyTabID)
	}
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		return nil, "", err
	}
	return ctx, resolvedID, nil
}

// HandleClipboardRead reads text from the browser clipboard.
func (h *Handlers) HandleClipboardRead(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	ctx, resolvedID, err := h.resolveClipboardTab(r, "")
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()

	var text string
	if err := chromedp.Run(tCtx, evalAwaitPromiseWithGesture(`(async () => navigator.clipboard.readText())()`, &text)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("clipboard read: %w", err))
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}

// HandleClipboardWrite writes text to the browser clipboard.
func (h *Handlers) HandleClipboardWrite(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req clipboardRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Text == nil {
		httpx.Error(w, http.StatusBadRequest, fmt.Errorf("text required"))
		return
	}

	ctx, resolvedID, err := h.resolveClipboardTab(r, req.TabID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()

	jsText, _ := json.Marshal(*req.Text)
	expr := fmt.Sprintf(`(async () => { await navigator.clipboard.writeText(%s); return true; })()`, jsText)
	var ok bool
	if err := chromedp.Run(tCtx, evalAwaitPromiseWithGesture(expr, &ok)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("clipboard write: %w", err))
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tabId":   resolvedID,
	})
}

// HandleClipboardCopy is an alias for HandleClipboardWrite.
func (h *Handlers) HandleClipboardCopy(w http.ResponseWriter, r *http.Request) {
	h.HandleClipboardWrite(w, r)
}

// HandleClipboardPaste reads from clipboard (paste = read clipboard content).
func (h *Handlers) HandleClipboardPaste(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req clipboardRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req)

	ctx, resolvedID, err := h.resolveClipboardTab(r, req.TabID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()

	var text string
	if err := chromedp.Run(tCtx, evalAwaitPromiseWithGesture(`(async () => navigator.clipboard.readText())()`, &text)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("clipboard paste: %w", err))
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}
