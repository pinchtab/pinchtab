package handlers

import (
	"context"
	"net/http"
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

// HandleGetBox returns the bounding box of an element identified by a unified
// selector (ref/css/xpath/text/semantic).
//
// @Endpoint GET /box
func (h *Handlers) HandleGetBox(w http.ResponseWriter, r *http.Request) {
	h.serveElementInspection(w, r, "inspect.box", func(ctx context.Context, tabID, sel string) (any, error) {
		box, err := h.getElementBox(ctx, tabID, sel)
		if err != nil {
			return nil, err
		}
		return boxResponse{Ref: sel, Box: *box}, nil
	})
}

// HandleTabGetBox returns the bounding box for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/box
func (h *Handlers) HandleTabGetBox(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetBox)
}

// getElementBox resolves a unified selector to a DOM node and returns its bounding client rect.
func (h *Handlers) getElementBox(ctx context.Context, tabID, sel string) (*boundingBox, error) {
	nodeID, err := h.resolveElementNodeID(ctx, tabID, sel)
	if err != nil {
		return nil, err
	}

	var result boundingBox
	err = h.Bridge.CallFunctionOnNode(ctx, nodeID,
		`function() { var r = this.getBoundingClientRect(); return {x: r.x, y: r.y, width: r.width, height: r.height, top: r.top, right: r.right, bottom: r.bottom, left: r.left}; }`,
		nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
