package handlers

import (
	"context"
	"net/http"
)

type valueResponse struct {
	Ref   string  `json:"ref"`
	Value *string `json:"value"` // null when element has no .value property
}

// HandleGetValue returns the current .value of a form element identified by a
// unified selector (ref/css/xpath/text/semantic).
//
// @Endpoint GET /value
func (h *Handlers) HandleGetValue(w http.ResponseWriter, r *http.Request) {
	h.serveElementInspection(w, r, "inspect.value", func(ctx context.Context, tabID, sel string) (any, error) {
		val, err := h.getElementValue(ctx, tabID, sel)
		if err != nil {
			return nil, err
		}
		return valueResponse{Ref: sel, Value: val}, nil
	})
}

// HandleTabGetValue returns the .value for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/value
func (h *Handlers) HandleTabGetValue(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetValue)
}

// getElementValue resolves a unified selector to a DOM node and returns its .value
// property. Returns a nil pointer when the element has no value property.
func (h *Handlers) getElementValue(ctx context.Context, tabID, sel string) (*string, error) {
	return callOnResolvedElement[*string](h, ctx, tabID, sel,
		`function() { return this.value !== undefined ? String(this.value) : null; }`, nil)
}
