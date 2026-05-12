package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

// cacheActionIntent stores the element's semantic identity in the
// IntentCache so the recovery engine can reconstruct a query if the
// ref becomes stale.
func (h *Handlers) cacheActionIntent(tabID string, req bridge.ActionRequest) {
	if h.Recovery == nil || req.Ref == "" {
		return
	}
	// Don't overwrite an existing entry that has a real Query (from /find)
	// with a descriptor-only entry.
	if existing, ok := h.Recovery.IntentCache.Lookup(tabID, req.Ref); ok && existing.Query != "" {
		return
	}
	desc := semantic.ElementDescriptor{Ref: req.Ref}
	// Try to enrich from the current snapshot cache.
	if cache := h.Bridge.GetRefCache(tabID); cache != nil {
		for _, enriched := range semanticDescriptorsFromNodes(cache.Nodes) {
			if enriched.Ref == req.Ref {
				desc = enriched
				break
			}
		}
	}
	h.Recovery.RecordIntent(tabID, req.Ref, recovery.IntentEntry{
		Descriptor: desc,
		CachedAt:   time.Now(),
	})
}

func (h *Handlers) executeAction(ctx context.Context, req bridge.ActionRequest) (map[string]any, string, error) {
	req.Kind = bridge.CanonicalActionKind(req.Kind)
	if h.shouldUseLiteAction(req) {
		return h.executeLiteAction(ctx, req)
	}

	if err := h.ensureChrome(); err != nil {
		return nil, "", fmt.Errorf("chrome initialization: %w", err)
	}
	result, err := h.Bridge.ExecuteAction(ctx, req.Kind, req)
	return result, "", err
}

func switchedTabFromActionResult(result map[string]any) string {
	if switched, ok := result["switchedToTab"].(string); ok {
		return strings.TrimSpace(switched)
	}
	return ""
}

func (h *Handlers) shouldUseLiteAction(req bridge.ActionRequest) bool {
	kind := bridge.CanonicalActionKind(req.Kind)
	if h.effectiveActionHumanize(req) && (kind == bridge.ActionClick || kind == bridge.ActionType || kind == bridge.ActionKeyboardType) {
		return false
	}
	capability, ok := actionCapability(kind)
	if !ok {
		return h.Router != nil && h.Router.Mode() == engine.ModeLite
	}
	return h.useLite(capability, "")
}

func (h *Handlers) effectiveActionHumanize(req bridge.ActionRequest) bool {
	if req.Humanize != nil {
		return *req.Humanize
	}
	if h != nil && h.Config != nil {
		return h.Config.Humanize
	}
	return false
}

func (h *Handlers) executeLiteAction(ctx context.Context, req bridge.ActionRequest) (map[string]any, string, error) {
	if h.Router == nil || h.Router.Lite() == nil {
		return nil, "", fmt.Errorf("lite engine unavailable")
	}
	switch bridge.CanonicalActionKind(req.Kind) {
	case bridge.ActionClick:
		if req.Ref == "" {
			return nil, "lite", fmt.Errorf("lite mode actions require ref from /snapshot")
		}
		if err := h.Router.Lite().Click(ctx, req.TabID, req.Ref); err != nil {
			return nil, "lite", err
		}
		return map[string]any{"clicked": true}, "lite", nil
	case bridge.ActionType, bridge.ActionFill:
		if req.Ref == "" {
			return nil, "lite", fmt.Errorf("lite mode actions require ref from /snapshot")
		}
		text := req.Text
		if req.Kind == bridge.ActionFill && text == "" {
			text = req.Value
		}
		if text == "" {
			return nil, "lite", fmt.Errorf("text required for %s", req.Kind)
		}
		if err := h.Router.Lite().Type(ctx, req.TabID, req.Ref, text); err != nil {
			return nil, "lite", err
		}
		return map[string]any{"typed": text}, "lite", nil
	default:
		return nil, "lite", fmt.Errorf("%w: %s", engine.ErrLiteNotSupported, req.Kind)
	}
}

func actionCapability(kind string) (engine.Capability, bool) {
	switch bridge.CanonicalActionKind(kind) {
	case bridge.ActionClick:
		return engine.CapClick, true
	case bridge.ActionType, bridge.ActionFill:
		return engine.CapType, true
	default:
		return "", false
	}
}

const pointerRetryDelay = 50 * time.Millisecond

func shouldRetryPointerAction(req bridge.ActionRequest, err error) bool {
	if err == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	switch kind {
	case bridge.ActionClick, bridge.ActionDoubleClick, bridge.ActionHover, bridge.ActionDrag,
		bridge.ActionMouseDown, bridge.ActionMouseUp, bridge.ActionMouseWheel:
		// pointer action kinds
	default:
		return false
	}

	if errors.Is(err, bridge.ErrElementOccluded) ||
		errors.Is(err, bridge.ErrElementBlocked) ||
		errors.Is(err, bridge.ErrElementOffscreen) {
		return true
	}

	return shouldRetryStaleRef(err)
}

func (h *Handlers) refreshActionNodeIDFromSelector(ctx context.Context, req *bridge.ActionRequest) {
	if req == nil || req.NodeID > 0 || strings.TrimSpace(req.Selector) == "" {
		return
	}
	nid, err := bridge.ResolveCSSToNodeID(ctx, req.Selector)
	if err != nil {
		return
	}
	req.NodeID = nid
}

func shouldRetryStaleRef(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, bridge.ErrElementStale) {
		return true
	}
	// Fallback string matching is still needed for stale failures that can bypass
	// bridge.ExecuteAction classification (for example, lite-engine paths or other
	// non-bridge error surfaces that return raw backend-node messages).
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "could not find node") || strings.Contains(e, "node with given id") || strings.Contains(e, "no node")
}

func (h *Handlers) refreshRefCache(ctx context.Context, tabID string) {
	nodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		return
	}
	flat, refs := bridge.BuildSnapshot(nodes, bridge.FilterInteractive, -1)
	_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, flat)
	h.Bridge.SetRefCache(tabID, &bridge.RefCache{
		Refs:    refs,
		Targets: bridge.RefTargetsFromNodes(flat),
		Nodes:   flat,
	})
}

func isClickTimeoutWithPendingDialog(err error, kind, tabID string, b bridge.BridgeAPI) bool {
	if err == nil || tabID == "" {
		return false
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	kind = bridge.CanonicalActionKind(kind)
	if kind != bridge.ActionClick && kind != bridge.ActionDoubleClick {
		return false
	}
	dm := b.GetDialogManager()
	if dm == nil {
		return false
	}
	return dm.GetPending(tabID) != nil
}
