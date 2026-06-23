package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/semantic"
)

func (h *Handlers) applySemanticActionSelector(ctx context.Context, tabID string, sel selector.Selector, req *bridge.ActionRequest) (bool, error) {
	return h.applySemanticActionSelectorInFrame(ctx, tabID, h.selectorFrameID(tabID), sel, req)
}

func (h *Handlers) applySemanticActionSelectorInFrame(ctx context.Context, tabID, frameID string, sel selector.Selector, req *bridge.ActionRequest) (bool, error) {
	query, ok := sel.SemanticQuery()
	if !ok {
		return false, nil
	}
	if h.Matcher == nil {
		return true, fmt.Errorf("semantic selectors require a matcher (not configured)")
	}

	descs, err := h.semanticDescriptorsForTabInFrame(ctx, tabID, frameID)
	if err != nil {
		return true, err
	}
	result, err := h.Matcher.Find(ctx, query, descs, semantic.FindOptions{
		Threshold: 0.3,
		TopK:      1,
	})
	if err != nil {
		return true, fmt.Errorf("semantic selector: %w", err)
	}
	if result.BestRef == "" {
		return true, fmt.Errorf("semantic selector %q: no matching element found", query)
	}

	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		return true, fmt.Errorf("semantic selector %q: no snapshot cache available", query)
	}
	target, ok := cache.Lookup(result.BestRef)
	if !ok || target.BackendNodeID == 0 {
		return true, fmt.Errorf("semantic selector %q matched ref %s but no node is available", query, result.BestRef)
	}

	req.Ref = result.BestRef
	req.NodeID = target.BackendNodeID
	req.Selector = ""
	return true, nil
}

func (h *Handlers) semanticDescriptorsForTabInFrame(ctx context.Context, tabID, frameID string) ([]semantic.ElementDescriptor, error) {
	nodes := h.resolveOrRefreshSnapshotNodes(ctx, tabID)
	if len(nodes) == 0 {
		return nil, fmt.Errorf("semantic selector: no snapshot available; navigate or snapshot first")
	}

	if cache := h.Bridge.GetRefCache(tabID); cache != nil && len(cache.Nodes) > 0 {
		_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, cache.Nodes)
		nodes = cache.Nodes
	}
	nodes = scopeSemanticNodesByFrame(nodes, frameID)
	if len(nodes) == 0 {
		return nil, fmt.Errorf("semantic selector: no elements found in current frame")
	}
	return semanticDescriptorsFromNodes(nodes), nil
}

func scopeSemanticNodesByFrame(nodes []bridge.A11yNode, frameID string) []bridge.A11yNode {
	if frameID == "" {
		return nodes
	}
	filtered := make([]bridge.A11yNode, 0, len(nodes))
	for _, node := range nodes {
		if node.FrameID == frameID {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func semanticSelectorHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not configured"):
		return http.StatusNotImplemented
	case strings.Contains(msg, "no matching element found"),
		strings.Contains(msg, "no node is available"):
		return http.StatusNotFound
	case strings.Contains(msg, "no snapshot available"),
		strings.Contains(msg, "no elements found"):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
