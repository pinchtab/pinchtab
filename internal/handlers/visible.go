package handlers

import (
	"context"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// visibleResponse answers two different questions about one element. Visible is
// CSS rendered-ness — display, visibility, opacity, a positioned or laid-out box
// with non-zero size — and scroll position is not an input, so an element far
// below the fold is still visible:true. OnScreen is viewport intersection, the
// same predicate the capture snapshot's per-node visible field publishes.
// OnScreen is absent when the element could not be measured, matching the
// capture rule that a missing answer means "not measured", never "no".
type visibleResponse struct {
	Ref      string `json:"ref"`
	Visible  bool   `json:"visible"`
	OnScreen *bool  `json:"onScreen,omitempty"`
}

// HandleGetVisible reports whether an element identified by a unified selector
// (ref/css/xpath/text/semantic) is rendered, plus whether it is on screen.
//
// @Endpoint GET /visible
func (h *Handlers) HandleGetVisible(w http.ResponseWriter, r *http.Request) {
	h.serveElementInspection(w, r, "inspect.visible", func(ctx context.Context, tabID, sel string) (any, error) {
		nodeID, err := h.resolveElementNodeID(ctx, tabID, sel)
		if err != nil {
			return nil, err
		}
		rendered, err := h.elementRendered(ctx, nodeID)
		if err != nil {
			return nil, err
		}
		return visibleResponse{Ref: sel, Visible: rendered, OnScreen: elementOnScreen(ctx, nodeID)}, nil
	})
}

// HandleTabGetVisible returns visibility for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/visible
func (h *Handlers) HandleTabGetVisible(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetVisible)
}

const elementVisibleJS = `function() {
  var el = this;
  var rect = el.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return false;
  if (el.checkVisibility) return el.checkVisibility({opacityProperty: true, visibilityProperty: true, checkOpacity: true, checkVisibilityCSS: true});
  var style = window.getComputedStyle(el);
  var isRoot = el === document.body || el === document.documentElement;
  if (!el.offsetParent && !isRoot && style.position !== 'fixed' && style.position !== 'sticky') return false;
  return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
}`

func (h *Handlers) elementRendered(ctx context.Context, nodeID int64) (bool, error) {
	var rendered bool
	err := h.Bridge.CallFunctionOnNode(ctx, nodeID, elementVisibleJS, nil, &rendered)
	return rendered, err
}

// elementOnScreen measures through the same border-box and viewport pair the
// capture path uses and asks bridge.IsOnScreen, so this endpoint and the capture
// snapshot cannot drift apart on the on-screen question. nil when the element
// has no box model or the layout read fails: unknown, not false.
func elementOnScreen(ctx context.Context, nodeID int64) *bool {
	box, ok := bridge.ElementBorderBox(ctx, nodeID)
	if !ok {
		return nil
	}
	vp, err := bridge.FetchLayout(ctx)
	if err != nil {
		return nil
	}
	onScreen := bridge.IsOnScreen(box, vp)
	return &onScreen
}
