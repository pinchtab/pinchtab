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
	"github.com/pinchtab/pinchtab/internal/web"
)

type clipboardRequest struct {
	TabID string  `json:"tabId"`
	Text  *string `json:"text"`
}

func evalAwaitPromise(expression string, res any) chromedp.Action {
	return chromedp.Evaluate(expression, res, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	})
}

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

func (h *Handlers) HandleClipboardRead(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	ctx, resolvedID, err := h.resolveClipboardTab(r, "")
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if err := grantClipboardPermissions(tCtx); err != nil {
		web.Error(w, 500, fmt.Errorf("grant clipboard permission: %w", err))
		return
	}

	var text string
	if err := chromedp.Run(tCtx, evalAwaitPromise(`(async () => navigator.clipboard.readText())()`, &text)); err != nil {
		web.Error(w, 500, fmt.Errorf("clipboard read: %w", err))
		return
	}

	web.JSON(w, 200, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}

func (h *Handlers) HandleClipboardWrite(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req clipboardRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Text == nil {
		web.Error(w, 400, fmt.Errorf("text required"))
		return
	}

	ctx, resolvedID, err := h.resolveClipboardTab(r, req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if err := grantClipboardPermissions(tCtx); err != nil {
		web.Error(w, 500, fmt.Errorf("grant clipboard permission: %w", err))
		return
	}

	jsText, _ := json.Marshal(*req.Text)
	expr := fmt.Sprintf(`(async () => { await navigator.clipboard.writeText(%s); return true; })()`, jsText)
	var ok bool
	if err := chromedp.Run(tCtx, evalAwaitPromise(expr, &ok)); err != nil {
		web.Error(w, 500, fmt.Errorf("clipboard write: %w", err))
		return
	}

	web.JSON(w, 200, map[string]any{
		"success": true,
		"tabId":   resolvedID,
	})
}

func (h *Handlers) HandleClipboardCopy(w http.ResponseWriter, r *http.Request) {
	h.HandleClipboardWrite(w, r)
}

func (h *Handlers) HandleClipboardPaste(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req clipboardRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req)

	ctx, resolvedID, err := h.resolveClipboardTab(r, req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if err := grantClipboardPermissions(tCtx); err != nil {
		web.Error(w, 500, fmt.Errorf("grant clipboard permission: %w", err))
		return
	}

	var text string
	if err := chromedp.Run(tCtx, evalAwaitPromise(`(async () => navigator.clipboard.readText())()`, &text)); err != nil {
		web.Error(w, 500, fmt.Errorf("clipboard paste: %w", err))
		return
	}

	web.JSON(w, 200, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}
