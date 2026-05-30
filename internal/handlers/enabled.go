package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type enabledResponse struct {
	Ref     string `json:"ref"`
	Enabled bool   `json:"enabled"`
}

// HandleGetEnabled returns whether an element identified by ref is enabled (not disabled).
//
// @Endpoint GET /enabled
func (h *Handlers) HandleGetEnabled(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.enabled", tabID)

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		httpx.Error(w, 400, fmt.Errorf("ref query parameter is required"))
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

	enabled, err := h.getElementEnabled(tCtx, resolvedTabID, ref)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	httpx.JSON(w, 200, enabledResponse{Ref: ref, Enabled: enabled})
}

// HandleTabGetEnabled returns enabled state for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/enabled
func (h *Handlers) HandleTabGetEnabled(w http.ResponseWriter, r *http.Request) {
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

	h.HandleGetEnabled(w, req)
}

// getElementEnabled resolves a ref to a DOM node and checks whether it is enabled.
func (h *Handlers) getElementEnabled(ctx context.Context, tabID, ref string) (bool, error) {
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

	var enabled bool
	err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
		`function() { return !this.disabled; }`,
		nil, &enabled)
	if err != nil {
		return false, err
	}
	return enabled, nil
}
