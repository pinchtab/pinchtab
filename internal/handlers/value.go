package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type valueResponse struct {
	Ref   string  `json:"ref"`
	Value *string `json:"value"` // null when element has no .value property
}

// HandleGetValue returns the current .value of a form element identified by ref.
//
// @Endpoint GET /value
func (h *Handlers) HandleGetValue(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.value", tabID)

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

	val, err := h.getElementValue(tCtx, resolvedTabID, ref)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	httpx.JSON(w, 200, valueResponse{Ref: ref, Value: val})
}

// HandleTabGetValue returns the .value for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/value
func (h *Handlers) HandleTabGetValue(w http.ResponseWriter, r *http.Request) {
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

	h.HandleGetValue(w, req)
}

// getElementValue resolves a ref to a DOM node and returns its .value property.
// Returns a nil pointer when the element has no value property.
func (h *Handlers) getElementValue(ctx context.Context, tabID, ref string) (*string, error) {
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		return nil, fmt.Errorf("ref not found: %s (no snapshot cache — run /snapshot first)", ref)
	}
	target, ok := cache.Lookup(ref)
	if !ok {
		return nil, fmt.Errorf("ref not found: %s", ref)
	}

	nodeID := target.BackendNodeID
	if nodeID == 0 {
		return nil, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	var result *string
	err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
		`function() { return this.value !== undefined ? String(this.value) : null; }`,
		nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
