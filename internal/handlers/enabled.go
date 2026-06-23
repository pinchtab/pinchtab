package handlers

import (
	"context"
	"net/http"
)

type enabledResponse struct {
	Ref     string `json:"ref"`
	Enabled bool   `json:"enabled"`
}

// HandleGetEnabled returns whether an element identified by a unified selector
// (ref/css/xpath/text/semantic) is enabled (not disabled).
//
// @Endpoint GET /enabled
func (h *Handlers) HandleGetEnabled(w http.ResponseWriter, r *http.Request) {
	h.serveElementInspection(w, r, "inspect.enabled", func(ctx context.Context, tabID, sel string) (any, error) {
		enabled, err := h.getElementEnabled(ctx, tabID, sel)
		if err != nil {
			return nil, err
		}
		return enabledResponse{Ref: sel, Enabled: enabled}, nil
	})
}

// HandleTabGetEnabled returns enabled state for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/enabled
func (h *Handlers) HandleTabGetEnabled(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetEnabled)
}

// getElementEnabled resolves a unified selector to a DOM node and checks whether it is enabled.
func (h *Handlers) getElementEnabled(ctx context.Context, tabID, sel string) (bool, error) {
	return callOnResolvedElement[bool](h, ctx, tabID, sel, `function() { return !this.disabled; }`, nil)
}
