package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type clipboardRequest struct {
	TabID string  `json:"tabId"`
	Text  *string `json:"text"`
}

// evalAwaitPromise wraps chromedp.Evaluate to await a promise result.
func evalAwaitPromise(expression string, res any) chromedp.Action {
	return chromedp.Evaluate(expression, res, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	})
}

// originFromURL extracts the origin (scheme://host) from a URL.
func originFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// grantClipboardPermissions grants clipboard read/write permissions for the current page origin.
func grantClipboardPermissions(ctx context.Context) error {
	var loc string
	if err := chromedp.Run(ctx, chromedp.Location(&loc)); err != nil {
		return err
	}
	origin := originFromURL(loc)
	if origin == "" {
		return nil
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return browser.GrantPermissions([]browser.PermissionType{
			browser.PermissionTypeClipboardReadWrite,
		}).WithOrigin(origin).Do(c)
	}))
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

	if err := grantClipboardPermissions(tCtx); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("grant clipboard permission: %w", err))
		return
	}

	var text string
	if err := chromedp.Run(tCtx, evalAwaitPromise(`(async () => navigator.clipboard.readText())()`, &text)); err != nil {
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

	if err := grantClipboardPermissions(tCtx); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("grant clipboard permission: %w", err))
		return
	}

	jsText, _ := json.Marshal(*req.Text)
	expr := fmt.Sprintf(`(async () => { await navigator.clipboard.writeText(%s); return true; })()`, jsText)
	var ok bool
	if err := chromedp.Run(tCtx, evalAwaitPromise(expr, &ok)); err != nil {
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

	if err := grantClipboardPermissions(tCtx); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("grant clipboard permission: %w", err))
		return
	}

	var text string
	if err := chromedp.Run(tCtx, evalAwaitPromise(`(async () => navigator.clipboard.readText())()`, &text)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("clipboard paste: %w", err))
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}
