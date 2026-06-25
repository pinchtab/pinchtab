package handlers

import (
	"context"
	"net/http"
)

type visibleResponse struct {
	Ref     string `json:"ref"`
	Visible bool   `json:"visible"`
}

// HandleGetVisible returns whether an element identified by a unified selector
// (ref/css/xpath/text/semantic) is visible on the page.
//
// @Endpoint GET /visible
func (h *Handlers) HandleGetVisible(w http.ResponseWriter, r *http.Request) {
	h.serveElementInspection(w, r, "inspect.visible", func(ctx context.Context, tabID, sel string) (any, error) {
		visible, err := h.getElementVisible(ctx, tabID, sel)
		if err != nil {
			return nil, err
		}
		return visibleResponse{Ref: sel, Visible: visible}, nil
	})
}

// HandleTabGetVisible returns visibility for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/visible
func (h *Handlers) HandleTabGetVisible(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetVisible)
}

// getElementVisible resolves a unified selector to a DOM node and checks whether it is visible.
func (h *Handlers) getElementVisible(ctx context.Context, tabID, sel string) (bool, error) {
	return callOnResolvedElement[bool](h, ctx, tabID, sel, `function() {
  var el = this;
  if (!el.offsetParent && el.style.position !== 'fixed' && el.style.position !== 'sticky') return false;
  var style = window.getComputedStyle(el);
  if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
  var rect = el.getBoundingClientRect();
  return rect.width > 0 && rect.height > 0;
}`, nil)
}
