package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type visibleResponse struct {
	Ref     string `json:"ref"`
	Visible bool   `json:"visible"`
}

// HandleGetVisible returns whether an element identified by ref is visible on the page.
//
// @Endpoint GET /visible
func (h *Handlers) HandleGetVisible(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.visible", tabID)

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

	visible, err := h.getElementVisible(tCtx, resolvedTabID, ref)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	httpx.JSON(w, 200, visibleResponse{Ref: ref, Visible: visible})
}

// HandleTabGetVisible returns visibility for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/visible
func (h *Handlers) HandleTabGetVisible(w http.ResponseWriter, r *http.Request) {
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

	h.HandleGetVisible(w, req)
}

// getElementVisible resolves a ref to a DOM node and checks whether it is visible.
func (h *Handlers) getElementVisible(ctx context.Context, tabID, ref string) (bool, error) {
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

	var visible bool
	err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
		`function() {
  var el = this;
  if (!el.offsetParent && el.style.position !== 'fixed' && el.style.position !== 'sticky') return false;
  var style = window.getComputedStyle(el);
  if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
  var rect = el.getBoundingClientRect();
  return rect.width > 0 && rect.height > 0;
}`,
		nil, &visible)
	if err != nil {
		return false, err
	}
	return visible, nil
}
