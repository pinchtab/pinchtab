package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/semantic"
	"github.com/pinchtab/pinchtab/internal/web"
)

func resolveOwner(r *http.Request, fallback string) string {
	if o := strings.TrimSpace(r.Header.Get("X-Owner")); o != "" {
		return o
	}
	if o := strings.TrimSpace(r.URL.Query().Get("owner")); o != "" {
		return o
	}
	return strings.TrimSpace(fallback)
}

func (h *Handlers) enforceTabLease(tabID, owner string) error {
	if tabID == "" {
		return nil
	}
	lock := h.Bridge.TabLockInfo(tabID)
	if lock == nil {
		return nil
	}
	if owner == "" {
		return fmt.Errorf("tab %s is locked by %s; owner required", tabID, lock.Owner)
	}
	if owner != lock.Owner {
		return fmt.Errorf("tab %s is locked by %s", tabID, lock.Owner)
	}
	return nil
}

// HandleAction performs a single action on a tab (click, type, fill, etc).
func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req bridge.ActionRequest
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.Kind = q.Get("kind")
		req.TabID = q.Get("tabId")
		req.Owner = q.Get("owner")
		req.Ref = q.Get("ref")
		req.Selector = q.Get("selector")
		req.Text = q.Get("text")
		req.Value = q.Get("value")
		req.Key = q.Get("key")
		if v := q.Get("nodeId"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				req.NodeID = n
			}
		}
	} else {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	// Validate kind — single endpoint returns 400 for bad input (unlike batch which returns 200 with errors)
	if req.Kind == "" {
		web.Error(w, 400, fmt.Errorf("missing required field 'kind'"))
		return
	}
	if available := h.Bridge.AvailableActions(); len(available) > 0 {
		known := false
		for _, k := range available {
			if k == req.Kind {
				known = true
				break
			}
		}
		if !known {
			web.Error(w, 400, fmt.Errorf("unknown action kind: %s", req.Kind))
			return
		}
	}

	// Resolve tab
	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	if req.TabID == "" {
		req.TabID = resolvedTabID
	}
	owner := resolveOwner(r, req.Owner)
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		web.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	// Allow custom timeout via query param (1-60 seconds)
	actionTimeout := h.Config.ActionTimeout
	if r.Method == http.MethodGet {
		if v := r.URL.Query().Get("timeout"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				if n > 0 && n <= 60 {
					actionTimeout = time.Duration(n * float64(time.Second))
				}
			}
		}
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	// Resolve ref → nodeID
	refMissing := false
	if req.Ref != "" && req.NodeID == 0 && req.Selector == "" {
		cache := h.Bridge.GetRefCache(resolvedTabID)
		if cache != nil {
			if nid, ok := cache.Refs[req.Ref]; ok {
				req.NodeID = nid
			}
		}
		if req.NodeID == 0 {
			refMissing = true
		}
	}

	// Cache intent before execution so recovery can reconstruct the query.
	// Only cache when the ref IS in the snapshot — otherwise we'd overwrite
	// the richer /find-cached entry (which has the Query) with a blank one.
	if req.Ref != "" && h.Recovery != nil && !refMissing {
		h.cacheActionIntent(resolvedTabID, req)
	}

	// If ref was not in snapshot cache, attempt semantic recovery before
	// returning 404. This handles the common case where a page reload
	// cleared the snapshot (DeleteRefCache) but the intent is still cached.
	var result map[string]any
	var actionErr error
	var recoveryResult *semantic.RecoveryResult

	if refMissing && req.Ref != "" && h.Recovery != nil {
		rr, actionRes, recoveryErr := h.Recovery.Attempt(
			tCtx, resolvedTabID, req.Ref, req.Kind,
			func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
				req.NodeID = nodeID
				return h.Bridge.ExecuteAction(ctx, kind, req)
			},
		)
		recoveryResult = &rr
		if recoveryErr == nil {
			result = actionRes
		} else {
			actionErr = fmt.Errorf("ref %s not found and recovery failed: %w", req.Ref, recoveryErr)
		}
	} else if refMissing {
		web.Error(w, 404, fmt.Errorf("ref %s not found - take a /snapshot first", req.Ref))
		return
	} else {
		result, actionErr = h.Bridge.ExecuteAction(tCtx, req.Kind, req)
		if actionErr != nil && req.Ref != "" && shouldRetryStaleRef(actionErr) {
			recordStaleRefRetry()
			h.refreshRefCache(tCtx, resolvedTabID)
			if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
				if nid, ok := cache.Refs[req.Ref]; ok {
					req.NodeID = nid
					result, actionErr = h.Bridge.ExecuteAction(tCtx, req.Kind, req)
				}
			}
		}
		// Semantic self-healing: if stale-ref retry still failed, attempt
		// recovery via the semantic matcher.
		if actionErr != nil && req.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(actionErr, req.Ref) {
			rr, actionRes, recoveryErr := h.Recovery.AttemptWithClassification(
				tCtx, resolvedTabID, req.Ref, req.Kind,
				semantic.ClassifyFailure(actionErr),
				func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
					req.NodeID = nodeID
					return h.Bridge.ExecuteAction(ctx, kind, req)
				},
			)
			recoveryResult = &rr
			if recoveryErr == nil {
				result = actionRes
				actionErr = nil
			}
		}
	}
	if actionErr != nil {
		if strings.HasPrefix(actionErr.Error(), "unknown action") {
			kinds := h.Bridge.AvailableActions()
			web.JSON(w, 400, map[string]string{
				"error": fmt.Sprintf("%s - valid values: %s", actionErr.Error(), strings.Join(kinds, ", ")),
			})
			return
		}
		web.ErrorCode(w, 500, "action_failed", fmt.Sprintf("action %s: %v", req.Kind, actionErr), true, nil)
		return
	}

	resp := map[string]any{"success": true, "result": result}
	if recoveryResult != nil {
		resp["recovery"] = recoveryResult
	}
	web.JSON(w, 200, resp)
}

// HandleTabAction performs a single action on a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/action
func (h *Handlers) HandleTabAction(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req bridge.ActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID != "" && req.TabID != tabID {
		web.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
		return
	}
	req.TabID = tabID

	payload, err := json.Marshal(req)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	wrapped := r.Clone(r.Context())
	wrapped.Body = io.NopCloser(bytes.NewReader(payload))
	wrapped.ContentLength = int64(len(payload))
	wrapped.Header = r.Header.Clone()
	wrapped.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, wrapped)
}

type actionsRequest struct {
	TabID       string                 `json:"tabId"`
	Owner       string                 `json:"owner"`
	Actions     []bridge.ActionRequest `json:"actions"`
	StopOnError bool                   `json:"stopOnError"`
}

type actionResult struct {
	Index   int            `json:"index"`
	Success bool           `json:"success"`
	Result  map[string]any `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func (h *Handlers) HandleActions(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if len(req.Actions) == 0 {
		web.Error(w, 400, fmt.Errorf("actions array is empty"))
		return
	}

	h.handleActionsBatch(w, r, req)
}

// HandleTabActions performs multiple actions on a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/actions
func (h *Handlers) HandleTabActions(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID != "" && req.TabID != tabID {
		web.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
		return
	}
	req.TabID = tabID

	payload, err := json.Marshal(req)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	wrapped := r.Clone(r.Context())
	wrapped.Body = io.NopCloser(bytes.NewReader(payload))
	wrapped.ContentLength = int64(len(payload))
	wrapped.Header = r.Header.Clone()
	wrapped.Header.Set("Content-Type", "application/json")
	h.HandleActions(w, wrapped)
}

// handleActionsBatch processes a batch of actions (used by both single and batch endpoints)
func (h *Handlers) handleActionsBatch(w http.ResponseWriter, r *http.Request, req actionsRequest) {

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	owner := resolveOwner(r, req.Owner)
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		web.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	results := make([]actionResult, 0, len(req.Actions))

	for i, action := range req.Actions {
		if action.TabID == "" {
			action.TabID = resolvedTabID
		} else if action.TabID != resolvedTabID {
			ctx, resolvedTabID, err = h.Bridge.TabContext(action.TabID)
			if err != nil {
				results = append(results, actionResult{
					Index: i, Success: false,
					Error: fmt.Sprintf("tab not found: %v", err),
				})
				if req.StopOnError {
					break
				}
				continue
			}
			if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
				results = append(results, actionResult{Index: i, Success: false, Error: err.Error()})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)

		if action.Ref != "" && action.NodeID == 0 && action.Selector == "" {
			cache := h.Bridge.GetRefCache(resolvedTabID)
			if cache != nil {
				if nid, ok := cache.Refs[action.Ref]; ok {
					action.NodeID = nid
				}
			}
		}

		refMissing := action.Ref != "" && action.NodeID == 0 && action.Selector == ""

		if action.Kind == "" {
			tCancel()
			results = append(results, actionResult{
				Index: i, Success: false, Error: "missing required field 'kind'",
			})
			if req.StopOnError {
				break
			}
			continue
		}

		// Cache intent before execution so recovery can reconstruct the query.
		// Only cache when the ref IS in the snapshot to avoid overwriting
		// the richer /find-cached entry (which has the Query).
		if action.Ref != "" && h.Recovery != nil && !refMissing {
			h.cacheActionIntent(resolvedTabID, action)
		}

		var actionRes map[string]any
		var err error

		if refMissing && h.Recovery != nil {
			// Ref not in snapshot cache but we may have a cached intent —
			// attempt semantic recovery (refresh snapshot + re-match).
			rr, recRes, recErr := h.Recovery.Attempt(
				tCtx, resolvedTabID, action.Ref, action.Kind,
				func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
					action.NodeID = nodeID
					return h.Bridge.ExecuteAction(ctx, kind, action)
				},
			)
			_ = rr
			if recErr == nil {
				actionRes = recRes
			} else {
				err = fmt.Errorf("ref %s not found and recovery failed: %w", action.Ref, recErr)
			}
		} else if refMissing {
			tCancel()
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: fmt.Sprintf("ref %s not found - take a /snapshot first", action.Ref),
			})
			if req.StopOnError {
				break
			}
			continue
		} else {
			actionRes, err = h.Bridge.ExecuteAction(tCtx, action.Kind, action)
			if err != nil && action.Ref != "" && shouldRetryStaleRef(err) {
				recordStaleRefRetry()
				h.refreshRefCache(tCtx, resolvedTabID)
				if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
					if nid, ok := cache.Refs[action.Ref]; ok {
						action.NodeID = nid
						actionRes, err = h.Bridge.ExecuteAction(tCtx, action.Kind, action)
					}
				}
			}
			// Semantic self-healing for batched actions.
			if err != nil && action.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, action.Ref) {
				rr, recRes, recErr := h.Recovery.AttemptWithClassification(
					tCtx, resolvedTabID, action.Ref, action.Kind,
					semantic.ClassifyFailure(err),
					func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
						action.NodeID = nodeID
						return h.Bridge.ExecuteAction(ctx, kind, action)
					},
				)
				_ = rr // recovery metadata not surfaced per-action in batch
				if recErr == nil {
					actionRes = recRes
					err = nil
				}
			}
		}
		tCancel()

		if err != nil {
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: fmt.Sprintf("action %s: %v", action.Kind, err),
			})
			if req.StopOnError {
				break
			}
		} else {
			results = append(results, actionResult{
				Index: i, Success: true, Result: actionRes,
			})
		}

		if i < len(req.Actions)-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	web.JSON(w, 200, map[string]any{
		"results":    results,
		"total":      len(req.Actions),
		"successful": countSuccessful(results),
		"failed":     len(req.Actions) - countSuccessful(results),
	})
}

func (h *Handlers) HandleMacro(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowMacro {
		web.ErrorCode(w, 403, "macro_disabled", web.DisabledEndpointMessage("macro", "security.allowMacro"), false, map[string]any{
			"setting": "security.allowMacro",
		})
		return
	}
	var req struct {
		TabID       string                 `json:"tabId"`
		Owner       string                 `json:"owner"`
		Steps       []bridge.ActionRequest `json:"steps"`
		StopOnError bool                   `json:"stopOnError"`
		StepTimeout float64                `json:"stepTimeout"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.ErrorCode(w, 400, "bad_request", fmt.Sprintf("decode: %v", err), false, nil)
		return
	}
	if len(req.Steps) == 0 {
		web.ErrorCode(w, 400, "bad_request", "steps array is empty", false, nil)
		return
	}
	owner := resolveOwner(r, req.Owner)
	stepTimeout := h.Config.ActionTimeout
	if req.StepTimeout > 0 && req.StepTimeout <= 60 {
		stepTimeout = time.Duration(req.StepTimeout * float64(time.Second))
	}

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		web.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	results := make([]actionResult, 0, len(req.Steps))
	for i, step := range req.Steps {
		if step.TabID == "" {
			step.TabID = resolvedTabID
		}
		// Resolve ref → nodeID from snapshot cache (mirrors HandleAction).
		stepRefMissing := false
		if step.Ref != "" && step.NodeID == 0 && step.Selector == "" {
			cache := h.Bridge.GetRefCache(resolvedTabID)
			if cache != nil {
				if nid, ok := cache.Refs[step.Ref]; ok {
					step.NodeID = nid
				}
			}
			if step.NodeID == 0 {
				stepRefMissing = true
			}
		}

		// Cache intent before execution so recovery can reconstruct the query.
		// Only cache when the ref IS in the snapshot to avoid overwriting
		// the richer /find-cached entry (which has the Query).
		if step.Ref != "" && h.Recovery != nil && !stepRefMissing {
			h.cacheActionIntent(resolvedTabID, step)
		}

		tCtx, cancel := context.WithTimeout(ctx, stepTimeout)

		var res map[string]any
		var err error

		if stepRefMissing && h.Recovery != nil {
			// Ref not in snapshot cache — attempt semantic recovery.
			rr, recRes, recErr := h.Recovery.Attempt(
				tCtx, resolvedTabID, step.Ref, step.Kind,
				func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
					step.NodeID = nodeID
					return h.Bridge.ExecuteAction(ctx, kind, step)
				},
			)
			_ = rr
			if recErr == nil {
				res = recRes
			} else {
				err = fmt.Errorf("ref %s not found and recovery failed: %w", step.Ref, recErr)
			}
		} else if stepRefMissing {
			cancel()
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: fmt.Sprintf("ref %s not found - take a /snapshot first", step.Ref),
			})
			if req.StopOnError {
				break
			}
			continue
		} else {
			res, err = h.Bridge.ExecuteAction(tCtx, step.Kind, step)
			if err != nil && step.Ref != "" && shouldRetryStaleRef(err) {
				recordStaleRefRetry()
				h.refreshRefCache(tCtx, resolvedTabID)
				if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
					if nid, ok := cache.Refs[step.Ref]; ok {
						step.NodeID = nid
						res, err = h.Bridge.ExecuteAction(tCtx, step.Kind, step)
					}
				}
			}
			// Semantic self-healing for macro steps.
			if err != nil && step.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, step.Ref) {
				rr, recRes, recErr := h.Recovery.AttemptWithClassification(
					tCtx, resolvedTabID, step.Ref, step.Kind,
					semantic.ClassifyFailure(err),
					func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
						step.NodeID = nodeID
						return h.Bridge.ExecuteAction(ctx, kind, step)
					},
				)
				_ = rr
				if recErr == nil {
					res = recRes
					err = nil
				}
			}
		}
		cancel()
		if err != nil {
			results = append(results, actionResult{Index: i, Success: false, Error: err.Error()})
			if req.StopOnError {
				break
			}
			continue
		}
		results = append(results, actionResult{Index: i, Success: true, Result: res})
	}

	web.JSON(w, 200, map[string]any{
		"kind":       "macro",
		"results":    results,
		"total":      len(req.Steps),
		"successful": countSuccessful(results),
		"failed":     len(req.Steps) - countSuccessful(results),
	})
}

func countSuccessful(results []actionResult) int {
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count
}

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
		for _, n := range cache.Nodes {
			if n.Ref == req.Ref {
				desc.Role = n.Role
				desc.Name = n.Name
				desc.Value = n.Value
				break
			}
		}
	}
	h.Recovery.RecordIntent(tabID, req.Ref, semantic.IntentEntry{
		Descriptor: desc,
		CachedAt:   time.Now(),
	})
}

func shouldRetryStaleRef(err error) bool {
	if err == nil {
		return false
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "could not find node") || strings.Contains(e, "node with given id") || strings.Contains(e, "no node")
}

func (h *Handlers) refreshRefCache(ctx context.Context, tabID string) {
	var rawResult json.RawMessage
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return chromedp.FromContext(c).Target.Execute(c, "Accessibility.getFullAXTree", nil, &rawResult)
		}),
	); err != nil {
		return
	}
	var treeResp struct {
		Nodes []bridge.RawAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		return
	}
	flat, refs := bridge.BuildSnapshot(treeResp.Nodes, bridge.FilterInteractive, -1)
	h.Bridge.SetRefCache(tabID, &bridge.RefCache{Refs: refs, Nodes: flat})
}
