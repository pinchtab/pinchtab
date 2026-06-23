package handlers

import (
	"context"
	"net/http"
)

type checkedResponse struct {
	Ref     string `json:"ref"`
	Checked bool   `json:"checked"`
}

// HandleGetChecked returns whether an element identified by a unified selector
// (ref/css/xpath/text/semantic) is checked.
//
// @Endpoint GET /checked
func (h *Handlers) HandleGetChecked(w http.ResponseWriter, r *http.Request) {
	h.serveElementInspection(w, r, "inspect.checked", func(ctx context.Context, tabID, sel string) (any, error) {
		checked, err := h.getElementChecked(ctx, tabID, sel)
		if err != nil {
			return nil, err
		}
		return checkedResponse{Ref: sel, Checked: checked}, nil
	})
}

// HandleTabGetChecked returns checked state for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/checked
func (h *Handlers) HandleTabGetChecked(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetChecked)
}

// getElementChecked resolves a unified selector to a DOM node and checks whether it is checked.
func (h *Handlers) getElementChecked(ctx context.Context, tabID, sel string) (bool, error) {
	return callOnResolvedElement[bool](h, ctx, tabID, sel, `function() { return !!this.checked; }`, nil)
}
