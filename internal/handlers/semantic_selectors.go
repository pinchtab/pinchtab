package handlers

import (
	"context"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/semantic"
)

func (h *Handlers) applySemanticActionSelectorInFrame(ctx context.Context, tabID, frameID string, sel selector.Selector, req *bridge.ActionRequest) (bool, error) {
	return h.applySemanticActionSelectorInScope(ctx, tabID, frameID, 0, sel, req)
}

func (h *Handlers) applySemanticActionSelectorInScope(ctx context.Context, tabID, frameID string, scopeBackendNodeID int64, sel selector.Selector, req *bridge.ActionRequest) (bool, error) {
	query, ok := sel.SemanticQuery()
	if !ok {
		return false, nil
	}
	if h.Matcher == nil {
		return true, ErrSemanticMatcherUnavailable
	}

	var (
		descs []semantic.ElementDescriptor
		nodes []bridge.A11yNode
		err   error
	)
	if scopeBackendNodeID != 0 {
		var rawNodes []bridge.RawAXNode
		rawNodes, err = bridge.FetchAXTree(ctx)
		if err == nil {
			foundScope := false
			for _, node := range rawNodes {
				if node.BackendDOMNodeID == scopeBackendNodeID {
					foundScope = true
					break
				}
			}
			if !foundScope {
				err = fmt.Errorf("semantic selector: topmost dialog is absent from the accessibility tree")
			} else {
				rawNodes = bridge.FilterSubtree(rawNodes, scopeBackendNodeID)
				nodes, _ = bridge.BuildSnapshot(rawNodes, "", -1)
				_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, nodes)
				descs = semanticDescriptorsFromNodes(nodes)
				if len(descs) == 0 {
					err = fmt.Errorf("semantic selector: no elements found in topmost dialog")
				}
			}
		}
	} else {
		descs, err = h.semanticDescriptorsForTabInFrame(ctx, tabID, frameID)
	}
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
		return true, h.emptySemanticMatchError(ctx, sel, query, descs)
	}

	if scopeBackendNodeID != 0 {
		for _, node := range nodes {
			if node.Ref == result.BestRef && node.NodeID != 0 {
				req.Ref = ""
				req.NodeID = node.NodeID
				req.Selector = ""
				return true, nil
			}
		}
		return true, fmt.Errorf("semantic selector %q matched ref %s but no dialog node is available", query, result.BestRef)
	}

	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		return true, fmt.Errorf("semantic selector %q: no snapshot cache available", query)
	}
	target, ok := cache.Lookup(result.BestRef)
	if !ok || target.BackendNodeID == 0 {
		// The snapshot named a ref the cache no longer holds: from the caller's side
		// that is a stale selector, not a broken server, so it carries the not-found
		// sentinel like the other misses.
		return true, fmt.Errorf("%w: semantic selector %q matched ref %s but no node is available", ErrElementNotFound, query, result.BestRef)
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

// emptySemanticMatchError distinguishes an empty page from an index nobody can satisfy.
// A positional nth: wrapper that runs off the end of the match set otherwise reports
// "no matching element found" while the elements are plainly there, which reads as a
// broken page rather than a bad index. The message names the index the CALLER wrote —
// zero-based, the spelling this project publishes — never the translated one the matcher
// received.
func (h *Handlers) emptySemanticMatchError(ctx context.Context, sel selector.Selector, query string, descs []semantic.ElementDescriptor) error {
	index, base, ok := sel.SemanticNthBase()
	if !ok {
		return fmt.Errorf("%w: semantic selector %q: no matching element found", ErrElementNotFound, query)
	}
	matches := h.countSemanticMatches(ctx, base, descs)
	if matches == 0 {
		return fmt.Errorf("%w: semantic selector %q: no matching element found", ErrElementNotFound, sel.String())
	}
	return fmt.Errorf("%w: semantic selector %q: index %d is out of range, %q matched %d element(s)", ErrElementNotFound, sel.String(), index, base, matches)
}

func (h *Handlers) countSemanticMatches(ctx context.Context, base string, descs []semantic.ElementDescriptor) int {
	result, err := h.Matcher.Find(ctx, base, descs, semantic.FindOptions{
		Threshold: 0.3,
		TopK:      len(descs),
	})
	if err != nil {
		return 0
	}
	return len(result.Matches)
}
