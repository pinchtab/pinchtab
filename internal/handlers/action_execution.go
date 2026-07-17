package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

const (
	postSubmitPollTimeout  = 3 * time.Second
	postSubmitPollInterval = 250 * time.Millisecond
)

var readTopmostSubmitModal = bridge.TopmostModalNodeIDInFrame

type submitStateSnapshot struct {
	URL        string `json:"url"`
	DialogOpen bool   `json:"dialogOpen"`
}

type submitPostState struct {
	Status         string              `json:"status"`
	Signal         string              `json:"signal"`
	Dispatch       string              `json:"dispatch"`
	ActionTimedOut bool                `json:"actionTimedOut"`
	ElapsedMS      int64               `json:"elapsedMs"`
	Before         submitStateSnapshot `json:"before"`
	After          submitStateSnapshot `json:"after"`
}

func (h *Handlers) captureSubmitState(ctx context.Context, tabID string) (submitStateSnapshot, error) {
	currentURL, err := h.Bridge.CurrentURL(ctx)
	if err != nil {
		return submitStateSnapshot{}, fmt.Errorf("read current URL: %w", err)
	}
	_, dialogOpen, err := readTopmostSubmitModal(ctx, h.selectorFrameID(tabID))
	if err != nil {
		return submitStateSnapshot{}, fmt.Errorf("read topmost dialog: %w", err)
	}
	return submitStateSnapshot{
		URL:        strings.TrimSpace(currentURL),
		DialogOpen: dialogOpen,
	}, nil
}

func submitTransitionSignal(before, after submitStateSnapshot) string {
	if before.URL != after.URL {
		return "url_changed"
	}
	if before.DialogOpen && !after.DialogOpen {
		return "dialog_closed"
	}
	return ""
}

func pendingSubmitPostState(before, after submitStateSnapshot, dispatch string, actionTimedOut bool, started time.Time) submitPostState {
	return submitPostState{
		Status:         "pending",
		Signal:         "no_terminal_change",
		Dispatch:       dispatch,
		ActionTimedOut: actionTimedOut,
		ElapsedMS:      time.Since(started).Milliseconds(),
		Before:         before,
		After:          after,
	}
}

// pollSubmitPostState observes only conservative terminal signals. The caller
// supplies a fresh bounded child of the live tab context, never the action
// context that may already have reached its deadline.
func (h *Handlers) pollSubmitPostState(ctx context.Context, tabID string, before submitStateSnapshot, dispatch string, actionTimedOut bool) (submitPostState, error) {
	started := time.Now()
	after := before
	observedPostState := false
	var lastReadErr error
	ticker := time.NewTicker(postSubmitPollInterval)
	defer ticker.Stop()

	for {
		observed, err := h.captureSubmitState(ctx, tabID)
		if err == nil {
			observedPostState = true
			lastReadErr = nil
			after = observed
			if signal := submitTransitionSignal(before, after); signal != "" {
				return submitPostState{
					Status:         "succeeded",
					Signal:         signal,
					Dispatch:       dispatch,
					ActionTimedOut: actionTimedOut,
					ElapsedMS:      time.Since(started).Milliseconds(),
					Before:         before,
					After:          after,
				}, nil
			}
		} else {
			lastReadErr = err
		}

		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return submitPostState{}, fmt.Errorf("post-submit observation canceled: %w", ctx.Err())
			}
			if !observedPostState {
				if lastReadErr == nil {
					lastReadErr = ctx.Err()
				}
				return submitPostState{}, fmt.Errorf("post-submit state was never observable: %w", lastReadErr)
			}
			if lastReadErr != nil && !errors.Is(lastReadErr, context.DeadlineExceeded) {
				return submitPostState{}, fmt.Errorf("post-submit observation failed: %w", lastReadErr)
			}
			return pendingSubmitPostState(before, after, dispatch, actionTimedOut, started), nil
		case <-ticker.C:
		}
	}
}

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
	if cache := h.Bridge.GetRefCache(tabID); cache != nil {
		for i := range cache.Nodes {
			if cache.Nodes[i].Ref == req.Ref {
				desc = descriptorFromNode(cache.Nodes[i])
				break
			}
		}
	}
	h.Recovery.RecordIntent(tabID, req.Ref, recovery.IntentEntry{
		Descriptor: desc,
		CachedAt:   time.Now(),
	})
}

func (h *Handlers) executeAction(ctx context.Context, req bridge.ActionRequest, cfg *config.RuntimeConfig) (map[string]any, string, error) {
	req.Kind = bridge.CanonicalActionKind(req.Kind)

	if err := h.ensureBrowser(cfg); err != nil {
		return nil, "", fmt.Errorf("browser initialization: %w", err)
	}
	result, err := h.Bridge.ExecuteAction(ctx, req.Kind, req)
	return result, "", err
}

// executeActionResilient runs one resolved action with pointer-retry, stale-ref
// cache refresh, and semantic self-healing. When refMissing is set it goes
// recovery-first (Recovery.Attempt); otherwise it executes then heals on
// failure. Callers must handle the refMissing-without-recovery case (404 /
// per-step failure) before calling — refMissing=true assumes h.Recovery != nil.
// req.NodeID is mutated in place, as the inline action/batch/macro paths did.
func (h *Handlers) executeActionResilient(ctx context.Context, req *bridge.ActionRequest, cfg *config.RuntimeConfig, resolvedTabID string, refMissing bool) (map[string]any, string, *recovery.RecoveryResult, error) {
	submitClick := bridge.IsSubmitClick(req.Kind, *req)
	if refMissing {
		if submitClick {
			return nil, "", nil, fmt.Errorf("click submit target is unresolved; automatic recovery is disabled")
		}
		rr, recRes, recErr := h.Recovery.Attempt(
			ctx, resolvedTabID, req.Ref, req.Kind,
			func(c context.Context, _ string, nodeID int64) (map[string]any, error) {
				req.NodeID = nodeID
				res, _, err := h.executeAction(c, *req, cfg)
				return res, err
			},
		)
		if recErr != nil {
			return nil, "", &rr, fmt.Errorf("ref %s not found and recovery failed: %w", req.Ref, recErr)
		}
		return recRes, "", &rr, nil
	}

	result, backend, err := h.executeAction(ctx, *req, cfg)
	if err != nil && !submitClick && shouldRetryPointerAction(*req, err) {
		if req.Ref != "" && shouldRetryStaleRef(err) {
			recordStaleRefRetry()
			h.refreshRefCache(ctx, resolvedTabID)
			if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
				if target, ok := cache.Lookup(req.Ref); ok {
					req.NodeID = target.BackendNodeID
				}
			}
		}
		h.refreshActionNodeIDFromSelector(ctx, req)
		time.Sleep(pointerRetryDelay)
		result, backend, err = h.executeAction(ctx, *req, cfg)
	}

	var rr *recovery.RecoveryResult
	if err != nil && !submitClick && req.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, req.Ref) {
		r2, recRes, recErr := h.Recovery.AttemptWithClassification(
			ctx, resolvedTabID, req.Ref, req.Kind,
			recovery.ClassifyFailure(err),
			func(c context.Context, _ string, nodeID int64) (map[string]any, error) {
				req.NodeID = nodeID
				res, _, e := h.executeAction(c, *req, cfg)
				return res, e
			},
		)
		rr = &r2
		if recErr == nil {
			result, err = recRes, nil
		}
	}
	return result, backend, rr, err
}

func switchedTabFromActionResult(result map[string]any) string {
	if switched, ok := result["switchedToTab"].(string); ok {
		return strings.TrimSpace(switched)
	}
	return ""
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
	// bridge.ExecuteAction classification (for example, static paths or other
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
