package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type boundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type boxResponse struct {
	Ref string      `json:"ref"`
	Box boundingBox `json:"box"`
}

// HandleGetBox returns the bounding box of an element identified by ref.
//
// @Endpoint GET /box
func (h *Handlers) HandleGetBox(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.box", tabID)

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

	box, err := h.getElementBox(tCtx, resolvedTabID, ref)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	httpx.JSON(w, 200, boxResponse{Ref: ref, Box: *box})
}

// HandleTabGetBox returns the bounding box for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/box
func (h *Handlers) HandleTabGetBox(w http.ResponseWriter, r *http.Request) {
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

	h.HandleGetBox(w, req)
}

// getElementBox resolves a ref to a DOM node and returns its bounding client rect.
func (h *Handlers) getElementBox(ctx context.Context, tabID, ref string) (*boundingBox, error) {
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

	var result boundingBox
	err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
		`function() { var r = this.getBoundingClientRect(); return {x: r.x, y: r.y, width: r.width, height: r.height, top: r.top, right: r.right, bottom: r.bottom, left: r.left}; }`,
		nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
