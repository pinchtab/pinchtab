package handlers

import (
	"context"
	"errors"
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

func (h *Handlers) resolveActionRequestSelector(ctx context.Context, tabID string, req *bridge.ActionRequest) (actionSelectorResolution, error) {
	original := *req
	original.NormalizeSelector()
	if original.NodeID != 0 {
		*req = original
		return actionSelectorResolution{}, nil
	}
	if original.Selector == "" {
		*req = original
		if original.Ref != "" {
			return actionSelectorResolution{refMissing: true}, nil
		}
		return actionSelectorResolution{}, nil
	}

	frameID := h.selectorFrameID(tabID)
	for attempt := 0; attempt < 2; attempt++ {
		modalNodeID, modalOpen, err := bridge.TopmostModalNodeID(ctx, frameID)
		if err != nil {
			return actionSelectorResolution{status: selectorFailureStatus(err)}, frameScopedSelectorError("topmost dialog", err)
		}

		attemptReq := original
		resolution, resolveErr := h.resolveActionRequestSelectorInScope(ctx, tabID, frameID, modalNodeID, modalOpen, &attemptReq)

		afterNodeID, afterOpen, scopeErr := bridge.TopmostModalNodeID(ctx, frameID)
		if scopeErr != nil {
			return actionSelectorResolution{status: selectorFailureStatus(scopeErr)}, frameScopedSelectorError("recheck topmost dialog", scopeErr)
		}
		if modalNodeID == afterNodeID && modalOpen == afterOpen {
			*req = attemptReq
			return resolution, resolveErr
		}
	}

	return actionSelectorResolution{status: http.StatusConflict}, fmt.Errorf("topmost dialog changed twice during selector resolution; retry after the page settles")
}

// resolveActionRequestDestination runs a scratch request through the source resolver so a
// drag destination accepts the same selector vocabulary, with the same frame and modal
// scoping, as the source.
func (h *Handlers) resolveActionRequestDestination(ctx context.Context, tabID string, req *bridge.ActionRequest) (actionSelectorResolution, error) {
	if req.ToSelector == "" || req.ToNodeID != 0 {
		return actionSelectorResolution{}, nil
	}
	destination := bridge.ActionRequest{Kind: req.Kind, TabID: req.TabID, Selector: req.ToSelector}
	resolution, err := h.resolveActionRequestSelector(ctx, tabID, &destination)
	if err != nil {
		return resolution, fmt.Errorf("drag destination: %w", err)
	}
	req.ToNodeID = destination.NodeID
	return resolution, nil
}

func (h *Handlers) resolveActionRequestSelectorInScope(
	ctx context.Context,
	tabID, frameID string,
	modalNodeID int64,
	modalOpen bool,
	req *bridge.ActionRequest,
) (actionSelectorResolution, error) {
	sel := selector.Parse(req.Selector)

	if handled, err := h.applySemanticActionSelectorInScope(ctx, tabID, frameID, modalNodeID, sel, req); handled {
		if err != nil {
			return actionSelectorResolution{status: selectorFailureStatus(err)}, err
		}
		return actionSelectorResolution{}, nil
	}
	resolve := func(sel selector.Selector) (int64, error) {
		if modalOpen {
			return bridge.ResolveUnifiedSelectorWithinNode(ctx, sel, h.Bridge.GetRefCache(tabID), modalNodeID)
		}
		return bridge.ResolveUnifiedSelectorInFrame(ctx, sel, h.Bridge.GetRefCache(tabID), frameID)
	}

	switch sel.Kind {
	case selector.KindRef:
		req.Ref = sel.Value
		req.Selector = ""
		nid, err := resolve(sel)
		if err != nil {
			if errors.Is(err, bridge.ErrSelectorNoMatch) {
				if modalOpen {
					return actionSelectorResolution{status: http.StatusNotFound}, frameScopedSelectorError("ref selector inside topmost dialog", err)
				}
				return actionSelectorResolution{refMissing: true}, nil
			}
			return actionSelectorResolution{status: selectorFailureStatus(err)}, frameScopedSelectorError("ref selector", err)
		}
		req.NodeID = nid
		if nid == 0 {
			return actionSelectorResolution{refMissing: true}, nil
		}
		if modalOpen {
			// The backend node is already validated against the active modal.
			// Clear the ref so stale-ref recovery cannot later refresh a global
			// snapshot and accidentally execute against a background ref with
			// the same ordinal.
			req.Ref = ""
		}
	case selector.KindCSS, selector.KindXPath, selector.KindText:
		nid, err := resolve(sel)
		if err != nil {
			return actionSelectorResolution{status: selectorFailureStatus(err)}, frameScopedSelectorError(string(sel.Kind)+" selector", err)
		}
		req.NodeID, req.Selector, req.Ref = nid, "", ""
	case selector.KindRole, selector.KindLabel, selector.KindPlaceholder,
		selector.KindAlt, selector.KindTitle, selector.KindTestID,
		selector.KindFirst, selector.KindLast, selector.KindNth:
		nid, err := resolve(sel)
		if err != nil {
			return actionSelectorResolution{status: selectorFailureStatus(err)}, frameScopedSelectorError("selector", err)
		}
		req.NodeID = nid
		req.Selector = ""
		req.Ref = ""
	case selector.KindSemantic:
		return actionSelectorResolution{status: http.StatusBadRequest}, fmt.Errorf("semantic selector requires a non-empty query")
	}

	return actionSelectorResolution{}, nil
}
