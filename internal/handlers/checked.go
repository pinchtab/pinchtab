package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type checkedResponse struct {
	Ref     string `json:"ref"`
	Checked bool   `json:"checked"`
}

// HandleGetChecked returns whether an element identified by ref is checked.
//
// @Endpoint GET /checked
func (h *Handlers) HandleGetChecked(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.checked", tabID)

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		httpx.Error(w, 400, fmt.Errorf("ref query parameter is required"))
		return
	}

	if err := h.ensureBrowser(h.Config); err != nil {
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

	checked, err := h.getElementChecked(tCtx, resolvedTabID, ref)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	httpx.JSON(w, 200, checkedResponse{Ref: ref, Checked: checked})
}

// HandleTabGetChecked returns checked state for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/checked
func (h *Handlers) HandleTabGetChecked(w http.ResponseWriter, r *http.Request) {
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

	h.HandleGetChecked(w, req)
}

// getElementChecked resolves a ref to a DOM node and checks whether it is checked.
func (h *Handlers) getElementChecked(ctx context.Context, tabID, ref string) (bool, error) {
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		return false, fmt.Errorf("ref not found: %s (no snapshot cache — run /snapshot first)", ref)
	}
	target, ok := cache.Lookup(ref)
	if !ok {
		return false, fmt.Errorf("ref not found: %s", ref)
	}

	nodeID := target.BackendNodeID
	if nodeID == 0 {
		return false, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	var checked bool
	err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
		`function() { return !!this.checked; }`,
		nil, &checked)
	if err != nil {
		return false, err
	}
	return checked, nil
}
