package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/selector"
)

type actionSelectorResolution struct {
	refMissing bool
	status     int
}

func (r actionSelectorResolution) httpStatus() int {
	if r.status != 0 {
		return r.status
	}
	return http.StatusBadRequest
}

func frameScopedSelectorError(kind string, err error) error {
	return fmt.Errorf("%s in current frame: %w", kind, err)
}

func (h *Handlers) resolveActionRequestSelector(ctx context.Context, tabID string, useStaticAction bool, req *bridge.ActionRequest) (actionSelectorResolution, error) {
	req.NormalizeSelector()
	if useStaticAction || req.NodeID != 0 {
		return actionSelectorResolution{}, nil
	}
	if req.Selector == "" {
		if req.Ref != "" {
			return actionSelectorResolution{refMissing: true}, nil
		}
		return actionSelectorResolution{}, nil
	}

	sel := selector.Parse(req.Selector)
	if handled, err := h.applySemanticActionSelector(ctx, tabID, sel, req); handled {
		if err != nil {
			return actionSelectorResolution{status: semanticSelectorHTTPStatus(err)}, err
		}
		return actionSelectorResolution{}, nil
	}

	switch sel.Kind {
	case selector.KindRef:
		req.Ref = sel.Value
		req.Selector = ""
		cache := h.Bridge.GetRefCache(tabID)
		if cache != nil {
			if target, ok := cache.Lookup(sel.Value); ok {
				req.NodeID = target.BackendNodeID
			}
		}
		if req.NodeID == 0 {
			return actionSelectorResolution{refMissing: true}, nil
		}
	case selector.KindCSS:
		req.Ref = ""
		nid, err := bridge.ResolveCSSToNodeIDInFrame(ctx, h.selectorFrameID(tabID), sel.Value)
		if err != nil {
			return actionSelectorResolution{}, frameScopedSelectorError("css selector", err)
		}
		req.NodeID = nid
		req.Selector = ""
	case selector.KindXPath:
		nid, err := bridge.ResolveXPathToNodeIDInFrame(ctx, h.selectorFrameID(tabID), sel.Value)
		if err != nil {
			return actionSelectorResolution{}, frameScopedSelectorError("xpath selector", err)
		}
		req.NodeID = nid
		req.Selector = ""
		req.Ref = ""
	case selector.KindText:
		nid, err := bridge.ResolveTextToNodeIDInFrame(ctx, h.selectorFrameID(tabID), sel.Value)
		if err != nil {
			return actionSelectorResolution{}, frameScopedSelectorError("text selector", err)
		}
		req.NodeID = nid
		req.Selector = ""
		req.Ref = ""
	case selector.KindRole, selector.KindLabel, selector.KindPlaceholder,
		selector.KindAlt, selector.KindTitle, selector.KindTestID,
		selector.KindFirst, selector.KindLast, selector.KindNth:
		nid, err := bridge.ResolveUnifiedSelectorInFrame(ctx, sel, h.Bridge.GetRefCache(tabID), h.selectorFrameID(tabID))
		if err != nil {
			return actionSelectorResolution{}, frameScopedSelectorError("selector", err)
		}
		req.NodeID = nid
		req.Selector = ""
		req.Ref = ""
	case selector.KindSemantic:
		return actionSelectorResolution{}, fmt.Errorf("semantic selector requires a non-empty query")
	}

	return actionSelectorResolution{}, nil
}
