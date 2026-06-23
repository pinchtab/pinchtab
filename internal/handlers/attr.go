package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type attrResponse struct {
	Ref   string  `json:"ref"`
	Name  string  `json:"name"`
	Value *string `json:"value"` // null when the attribute does not exist
}

// HandleGetAttr returns the value of a specific HTML attribute on an element
// identified by a unified selector (ref/css/xpath/text/semantic).
//
// @Endpoint GET /attr
func (h *Handlers) HandleGetAttr(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, "inspect.attr", tabID)

	sel := inspectSelectorParam(r)
	if sel == "" {
		httpx.Error(w, 400, fmt.Errorf("selector (or ref) query parameter is required"))
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		httpx.Error(w, 400, fmt.Errorf("name query parameter is required"))
		return
	}

	h.inspectElement(w, r, tabID, func(ctx context.Context, resolvedTabID string) (any, error) {
		val, err := h.getElementAttr(ctx, resolvedTabID, sel, name)
		if err != nil {
			return nil, err
		}
		return attrResponse{Ref: sel, Name: name, Value: val}, nil
	})
}

// HandleTabGetAttr returns the attribute value for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/attr
func (h *Handlers) HandleTabGetAttr(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetAttr)
}

// getElementAttr resolves a unified selector to a DOM node and returns the value
// of the named HTML attribute. Returns a nil pointer when the attribute does not exist.
func (h *Handlers) getElementAttr(ctx context.Context, tabID, sel, name string) (*string, error) {
	return callOnResolvedElement[*string](h, ctx, tabID, sel,
		`function(n) { var v = this.getAttribute(n); return v !== null ? v : null; }`,
		[]map[string]any{{"value": name}})
}
