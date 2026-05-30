package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type countResponse struct {
	Selector string `json:"selector"`
	Count    int    `json:"count"`
}

// HandleCount returns the number of elements matching a CSS selector.
//
// @Endpoint GET /count
func (h *Handlers) HandleCount(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.count", tabID)

	selector := r.URL.Query().Get("selector")
	if selector == "" {
		httpx.Error(w, 400, fmt.Errorf("selector query parameter is required"))
		return
	}

	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
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

	count, err := h.countElements(tCtx, selector)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("count elements: %w", err))
		return
	}

	httpx.JSON(w, 200, countResponse{Selector: selector, Count: count})
}

// HandleTabCount returns the element count for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/count
func (h *Handlers) HandleTabCount(w http.ResponseWriter, r *http.Request) {
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

	h.HandleCount(w, req)
}

// countElements evaluates document.querySelectorAll(selector).length in the page.
func (h *Handlers) countElements(ctx context.Context, selector string) (int, error) {
	selectorJSON, err := json.Marshal(selector)
	if err != nil {
		return 0, fmt.Errorf("encode selector: %w", err)
	}
	expr := fmt.Sprintf("document.querySelectorAll(%s).length", string(selectorJSON))

	var count int
	if err := h.evalRuntime(ctx, expr, &count, bridge.EvalOpts{}); err != nil {
		return 0, err
	}
	return count, nil
}
