package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// withPathTabID maps the {id} path value into the tabId query param and calls
// root with a cloned request; writes 400 when the path id is empty.
func (h *Handlers) withPathTabID(w http.ResponseWriter, r *http.Request, root http.HandlerFunc) {
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
	root(w, req)
}

// callOnResolvedElement resolves sel to a DOM node on tabID and runs jsFn on it,
// unmarshaling the result into T. args are forwarded to CallFunctionOnNode (nil
// for a no-arg function). It is the shared body of the element-property getters
// (getElementChecked/Enabled/Value/Visible/Attr) — after this, those differ only
// in the JS snippet, its args, and the result type. Any error yields the zero T.
func callOnResolvedElement[T any](h *Handlers, ctx context.Context, tabID, sel, jsFn string, args []map[string]any) (T, error) {
	var result T
	nodeID, err := h.resolveElementNodeID(ctx, tabID, sel)
	if err != nil {
		return result, err
	}
	if err := h.Bridge.CallFunctionOnNode(ctx, nodeID, jsFn, args, &result); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

// inspectSelectorParam reads the element selector, accepting the unified-selector
// `selector` param and falling back to the legacy `ref` alias for back-compat.
func inspectSelectorParam(r *http.Request) string {
	if sel := r.URL.Query().Get("selector"); sel != "" {
		return sel
	}
	return r.URL.Query().Get("ref")
}

// serveElementInspection runs the shared read-only single-element inspection
// preamble: record the read, require a unified selector, then resolve + inspect
// via inspectElement. build returns the JSON response for the resolved element;
// its error is mapped by inspectElement (statusForElementErr).
//
// attr and count do not use this: attr validates an extra required `name` param
// before tab resolution, and count targets multiple elements via a different
// selector/evaluation path.
func (h *Handlers) serveElementInspection(w http.ResponseWriter, r *http.Request, metric string,
	build func(ctx context.Context, tabID, sel string) (any, error)) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, metric, tabID)

	sel := inspectSelectorParam(r)
	if sel == "" {
		httpx.Error(w, 400, fmt.Errorf("selector (or ref) query parameter is required"))
		return
	}

	h.inspectElement(w, r, tabID, func(ctx context.Context, resolvedTabID string) (any, error) {
		return build(ctx, resolvedTabID, sel)
	})
}

// inspectElement runs the shared read-only inspect lifecycle (browser init, tab
// resolution, domain-policy enforcement, auto-close arming, and timeout/cancel
// wiring) and writes the JSON value fn returns, mapping fn's error via
// statusForElementErr. Handlers keep their own param parsing + recordReadRequest
// and pass the per-endpoint getter/response as fn.
func (h *Handlers) inspectElement(w http.ResponseWriter, r *http.Request, tabID string,
	fn func(ctx context.Context, resolvedTabID string) (any, error)) {
	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("browser initialization: %w", err))
		return
	}

	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	result, err := fn(tCtx, resolvedTabID)
	if err != nil {
		httpx.Error(w, statusForElementErr(err), err)
		return
	}
	httpx.JSON(w, 200, result)
}
