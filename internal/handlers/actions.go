package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
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
		if v := q.Get("x"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				req.X = n
				req.HasXY = true
			}
		}
		if v := q.Get("y"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				req.Y = n
				req.HasXY = true
			}
		}
		if v := q.Get("hasXY"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				req.HasXY = b
			}
		}
	} else {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	// Validate kind — single endpoint returns 400 for bad input (unlike batch which returns 200 with errors)
	if req.Kind == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'kind'"))
		return
	}
	h.recordActionRequest(r, req)
	if !h.shouldUseLiteAction(req.Kind) {
		if available := h.Bridge.AvailableActions(); len(available) > 0 {
			known := false
			for _, k := range available {
				if k == req.Kind {
					known = true
					break
				}
			}
			if !known {
				httpx.Error(w, 400, fmt.Errorf("unknown action kind: %s", req.Kind))
				return
			}
		}
	}

	// Resolve tab — skip for lite actions (lite engine manages its own tabs)
	useLiteAction := h.shouldUseLiteAction(req.Kind)
	var resolvedTabID string
	var ctx context.Context
	if useLiteAction {
		ctx = r.Context()
		resolvedTabID = req.TabID
	} else {
		var err error
		ctx, resolvedTabID, err = h.tabContext(r, req.TabID)
		if err != nil {
			httpx.Error(w, 404, err)
			return
		}
		if req.TabID == "" {
			req.TabID = resolvedTabID
		}
		owner := resolveOwner(r, req.Owner)
		if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
			httpx.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
			return
		}
		if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
			return
		}
	}
	h.recordResolvedTab(r, resolvedTabID)

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
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	// Unified selector resolution: normalize legacy ref/selector fields
	// into the unified Selector, then resolve to a nodeID when possible.
	req.NormalizeSelector()
	refMissing := false
	if !useLiteAction && req.NodeID == 0 && req.Selector != "" {
		sel := selector.Parse(req.Selector)
		switch sel.Kind {
		case selector.KindRef:
			// Ensure Ref is set for downstream recovery/intent caching.
			req.Ref = sel.Value
			// Clear Selector so the bridge doesn't try to use the ref
			// string as a CSS selector (it checks Selector before NodeID).
			req.Selector = ""
			cache := h.Bridge.GetRefCache(resolvedTabID)
			if cache != nil {
				if nid, ok := cache.Refs[sel.Value]; ok {
					req.NodeID = nid
				}
			}
			if req.NodeID == 0 {
				refMissing = true
			}
		case selector.KindCSS:
			// CSS selectors are resolved by the bridge action handlers directly
			// via chromedp, so we keep req.Selector as-is (the bridge checks
			// req.Selector before req.NodeID). Clear Ref so the bridge doesn't
			// confuse it with a snapshot ref.
			req.Ref = ""
			req.Selector = sel.Value
		case selector.KindXPath:
			nid, err := bridge.ResolveXPathToNodeID(tCtx, sel.Value)
			if err != nil {
				httpx.Error(w, 400, fmt.Errorf("xpath selector: %w", err))
				return
			}
			req.NodeID = nid
			req.Selector = ""
			req.Ref = ""
		case selector.KindText:
			nid, err := bridge.ResolveTextToNodeID(tCtx, sel.Value)
			if err != nil {
				httpx.Error(w, 400, fmt.Errorf("text selector: %w", err))
				return
			}
			req.NodeID = nid
			req.Selector = ""
			req.Ref = ""
		case selector.KindSemantic:
			// Semantic selectors require the matcher — resolve via find logic.
			if h.Matcher != nil {
				nodes := h.resolveSnapshotNodes(resolvedTabID)
				if len(nodes) == 0 {
					h.refreshRefCache(tCtx, resolvedTabID)
					nodes = h.resolveSnapshotNodes(resolvedTabID)
				}
				if len(nodes) > 0 {
					descs := make([]semantic.ElementDescriptor, len(nodes))
					for i, n := range nodes {
						descs[i] = semantic.ElementDescriptor{
							Ref: n.Ref, Role: n.Role, Name: n.Name, Value: n.Value,
						}
					}
					result, err := h.Matcher.Find(tCtx, sel.Value, descs, semantic.FindOptions{
						Threshold: 0.3, TopK: 1,
					})
					if err != nil {
						httpx.Error(w, 500, fmt.Errorf("semantic selector: %w", err))
						return
					}
					if result.BestRef != "" {
						req.Ref = result.BestRef
						cache := h.Bridge.GetRefCache(resolvedTabID)
						if cache != nil {
							if nid, ok := cache.Refs[result.BestRef]; ok {
								req.NodeID = nid
							}
						}
					}
					if req.NodeID == 0 {
						httpx.Error(w, 404, fmt.Errorf("semantic selector %q: no matching element found", sel.Value))
						return
					}
				} else {
					httpx.Error(w, 500, fmt.Errorf("semantic selector: no snapshot available — navigate first"))
					return
				}
			} else {
				httpx.Error(w, 501, fmt.Errorf("semantic selectors require a matcher (not configured)"))
				return
			}
			req.Selector = ""
		}
	}

	// Cache intent before execution so recovery can reconstruct the query.
	// Only cache when the ref IS in the snapshot — otherwise we'd overwrite
	// the richer /find-cached entry (which has the Query) with a blank one.
	if !useLiteAction && req.Ref != "" && h.Recovery != nil && !refMissing {
		h.cacheActionIntent(resolvedTabID, req)
	}

	// If ref was not in snapshot cache, attempt semantic recovery before
	// returning 404. This handles the common case where a page reload
	// cleared the snapshot (DeleteRefCache) but the intent is still cached.
	var result map[string]any
	var engineName string
	var actionErr error
	var recoveryResult *recovery.RecoveryResult

	if refMissing && req.Ref != "" && h.Recovery != nil {
		rr, actionRes, recoveryErr := h.Recovery.Attempt(
			tCtx, resolvedTabID, req.Ref, req.Kind,
			func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
				req.NodeID = nodeID
				res, _, err := h.executeAction(ctx, req)
				return res, err
			},
		)
		recoveryResult = &rr
		if recoveryErr == nil {
			result = actionRes
		} else {
			actionErr = fmt.Errorf("ref %s not found and recovery failed: %w", req.Ref, recoveryErr)
		}
	} else if refMissing {
		httpx.Error(w, 404, fmt.Errorf("ref %s not found - take a /snapshot first", req.Ref))
		return
	} else {
		result, engineName, actionErr = h.executeAction(tCtx, req)
		if actionErr != nil && req.Ref != "" && shouldRetryStaleRef(actionErr) {
			recordStaleRefRetry()
			h.refreshRefCache(tCtx, resolvedTabID)
			if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
				if nid, ok := cache.Refs[req.Ref]; ok {
					req.NodeID = nid
					result, engineName, actionErr = h.executeAction(tCtx, req)
				}
			}
		}
		// Semantic self-healing: if stale-ref retry still failed, attempt
		// recovery via the semantic matcher.
		if actionErr != nil && req.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(actionErr, req.Ref) {
			rr, actionRes, recoveryErr := h.Recovery.AttemptWithClassification(
				tCtx, resolvedTabID, req.Ref, req.Kind,
				recovery.ClassifyFailure(actionErr),
				func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
					req.NodeID = nodeID
					res, _, err := h.executeAction(ctx, req)
					return res, err
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
			httpx.JSON(w, 400, map[string]string{
				"error": fmt.Sprintf("%s - valid values: %s", actionErr.Error(), strings.Join(kinds, ", ")),
			})
			return
		}
		if errors.Is(actionErr, bridge.ErrUnexpectedNavigation) {
			httpx.ErrorCode(w, 409, "navigation_changed", actionErr.Error(), false, nil)
			return
		}
		if errors.Is(actionErr, engine.ErrLiteNotSupported) {
			httpx.ErrorCode(w, http.StatusNotImplemented, "not_supported", actionErr.Error(), false, nil)
			return
		}
		httpx.ErrorCode(w, 500, "action_failed", fmt.Sprintf("action %s: %v", req.Kind, actionErr), true, nil)
		return
	}

	if engineName == "lite" {
		w.Header().Set("X-Engine", "lite")
		h.recordEngine(r, "lite")
	}
	resp := map[string]any{"success": true, "result": result}
	if recoveryResult != nil {
		resp["recovery"] = recoveryResult
	}
	httpx.JSON(w, 200, resp)
}

// HandleTabAction performs a single action on a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/action
func (h *Handlers) HandleTabAction(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req bridge.ActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID != "" && req.TabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
		return
	}
	req.TabID = tabID

	payload, err := json.Marshal(req)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
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
	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if len(req.Actions) == 0 {
		httpx.Error(w, 400, fmt.Errorf("actions array is empty"))
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
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID != "" && req.TabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
		return
	}
	req.TabID = tabID

	payload, err := json.Marshal(req)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
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

	// Check if the first action is lite-routable to decide tab resolution strategy
	allLite := h.Router != nil && h.Router.Mode() == engine.ModeLite
	var ctx context.Context
	var resolvedTabID string
	owner := resolveOwner(r, req.Owner)
	if allLite {
		ctx = r.Context()
		resolvedTabID = req.TabID
	} else {
		var err error
		ctx, resolvedTabID, err = h.tabContext(r, req.TabID)
		if err != nil {
			httpx.Error(w, 404, err)
			return
		}
		if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
			httpx.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
			return
		}
	}

	results := make([]actionResult, 0, len(req.Actions))
	for i, action := range req.Actions {
		if action.TabID == "" {
			action.TabID = resolvedTabID
		} else if !allLite && action.TabID != resolvedTabID {
			var err error
			ctx, resolvedTabID, err = h.tabContext(r, action.TabID)
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
		if !allLite {
			if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
				return
			}
		}

		tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
		useLiteAction := h.shouldUseLiteAction(action.Kind)

		// Unified selector resolution for batch actions.
		action.NormalizeSelector()
		refMissing := false
		if !useLiteAction && action.NodeID == 0 && action.Selector != "" {
			sel := selector.Parse(action.Selector)
			switch sel.Kind {
			case selector.KindRef:
				action.Ref = sel.Value
				action.Selector = ""
				cache := h.Bridge.GetRefCache(resolvedTabID)
				if cache != nil {
					if nid, ok := cache.Refs[sel.Value]; ok {
						action.NodeID = nid
					}
				}
				if action.NodeID == 0 {
					refMissing = true
				}
			case selector.KindCSS:
				action.Ref = ""
				action.Selector = sel.Value
			case selector.KindXPath:
				nid, resolveErr := bridge.ResolveXPathToNodeID(tCtx, sel.Value)
				if resolveErr != nil {
					tCancel()
					results = append(results, actionResult{
						Index: i, Success: false,
						Error: fmt.Sprintf("xpath selector: %v", resolveErr),
					})
					if req.StopOnError {
						break
					}
					continue
				}
				action.NodeID = nid
				action.Selector = ""
				action.Ref = ""
			case selector.KindText:
				nid, resolveErr := bridge.ResolveTextToNodeID(tCtx, sel.Value)
				if resolveErr != nil {
					tCancel()
					results = append(results, actionResult{
						Index: i, Success: false,
						Error: fmt.Sprintf("text selector: %v", resolveErr),
					})
					if req.StopOnError {
						break
					}
					continue
				}
				action.NodeID = nid
				action.Selector = ""
				action.Ref = ""
			case selector.KindSemantic:
				if h.Matcher != nil {
					nodes := h.resolveSnapshotNodes(resolvedTabID)
					if len(nodes) == 0 {
						h.refreshRefCache(tCtx, resolvedTabID)
						nodes = h.resolveSnapshotNodes(resolvedTabID)
					}
					if len(nodes) > 0 {
						descs := make([]semantic.ElementDescriptor, len(nodes))
						for j, n := range nodes {
							descs[j] = semantic.ElementDescriptor{
								Ref: n.Ref, Role: n.Role, Name: n.Name, Value: n.Value,
							}
						}
						findResult, findErr := h.Matcher.Find(tCtx, sel.Value, descs, semantic.FindOptions{
							Threshold: 0.3, TopK: 1,
						})
						if findErr == nil && findResult.BestRef != "" {
							action.Ref = findResult.BestRef
							cache := h.Bridge.GetRefCache(resolvedTabID)
							if cache != nil {
								if nid, ok := cache.Refs[findResult.BestRef]; ok {
									action.NodeID = nid
								}
							}
						}
					}
					if action.NodeID == 0 {
						tCancel()
						results = append(results, actionResult{
							Index: i, Success: false,
							Error: fmt.Sprintf("semantic selector %q: no matching element found", sel.Value),
						})
						if req.StopOnError {
							break
						}
						continue
					}
				} else {
					tCancel()
					results = append(results, actionResult{
						Index: i, Success: false,
						Error: "semantic selectors require a matcher (not configured)",
					})
					if req.StopOnError {
						break
					}
					continue
				}
				action.Selector = ""
			}
		} else if !useLiteAction && action.Ref != "" && action.NodeID == 0 {
			// Legacy path: Ref set but NormalizeSelector didn't promote it
			// (shouldn't happen, but defensive).
			refMissing = true
		}

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
		if !useLiteAction && action.Ref != "" && h.Recovery != nil && !refMissing {
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
					res, _, err := h.executeAction(ctx, action)
					return res, err
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
			actionRes, _, err = h.executeAction(tCtx, action)
			if err != nil && action.Ref != "" && shouldRetryStaleRef(err) {
				recordStaleRefRetry()
				h.refreshRefCache(tCtx, resolvedTabID)
				if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
					if nid, ok := cache.Refs[action.Ref]; ok {
						action.NodeID = nid
						actionRes, _, err = h.executeAction(tCtx, action)
					}
				}
			}
			// Semantic self-healing for batched actions.
			if err != nil && action.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, action.Ref) {
				rr, recRes, recErr := h.Recovery.AttemptWithClassification(
					tCtx, resolvedTabID, action.Ref, action.Kind,
					recovery.ClassifyFailure(err),
					func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
						action.NodeID = nodeID
						res, _, err := h.executeAction(ctx, action)
						return res, err
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

	httpx.JSON(w, 200, map[string]any{
		"results":    results,
		"total":      len(req.Actions),
		"successful": countSuccessful(results),
		"failed":     len(req.Actions) - countSuccessful(results),
	})
}

func (h *Handlers) HandleMacro(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowMacro {
		httpx.ErrorCode(w, 403, "macro_disabled", httpx.DisabledEndpointMessage("macro", "security.allowMacro"), false, map[string]any{
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
		httpx.ErrorCode(w, 400, "bad_request", fmt.Sprintf("decode: %v", err), false, nil)
		return
	}
	if len(req.Steps) == 0 {
		httpx.ErrorCode(w, 400, "bad_request", "steps array is empty", false, nil)
		return
	}
	owner := resolveOwner(r, req.Owner)
	stepTimeout := h.Config.ActionTimeout
	if req.StepTimeout > 0 && req.StepTimeout <= 60 {
		stepTimeout = time.Duration(req.StepTimeout * float64(time.Second))
	}

	allLiteMacro := h.Router != nil && h.Router.Mode() == engine.ModeLite
	var ctx context.Context
	var resolvedTabID string
	if allLiteMacro {
		ctx = r.Context()
		resolvedTabID = req.TabID
	} else {
		var err error
		ctx, resolvedTabID, err = h.tabContext(r, req.TabID)
		if err != nil {
			httpx.Error(w, 404, err)
			return
		}
		if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
			httpx.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
			return
		}
	}

	results := make([]actionResult, 0, len(req.Steps))
	for i, step := range req.Steps {
		if step.TabID == "" {
			step.TabID = resolvedTabID
		}
		if !allLiteMacro {
			if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
				return
			}
		}
		useLiteAction := h.shouldUseLiteAction(step.Kind)
		// Unified selector resolution for macro steps (mirrors HandleAction).
		step.NormalizeSelector()
		stepRefMissing := false
		if !useLiteAction && step.NodeID == 0 && step.Selector != "" {
			sel := selector.Parse(step.Selector)
			switch sel.Kind {
			case selector.KindRef:
				step.Ref = sel.Value
				step.Selector = ""
				cache := h.Bridge.GetRefCache(resolvedTabID)
				if cache != nil {
					if nid, ok := cache.Refs[sel.Value]; ok {
						step.NodeID = nid
					}
				}
				if step.NodeID == 0 {
					stepRefMissing = true
				}
			case selector.KindCSS:
				step.Ref = ""
				step.Selector = sel.Value
			case selector.KindXPath:
				tCtx, cancel := context.WithTimeout(ctx, stepTimeout)
				nid, resolveErr := bridge.ResolveXPathToNodeID(tCtx, sel.Value)
				cancel()
				if resolveErr != nil {
					results = append(results, actionResult{
						Index: i, Success: false,
						Error: fmt.Sprintf("xpath selector: %v", resolveErr),
					})
					if req.StopOnError {
						break
					}
					continue
				}
				step.NodeID = nid
				step.Selector = ""
				step.Ref = ""
			case selector.KindText:
				tCtx, cancel := context.WithTimeout(ctx, stepTimeout)
				nid, resolveErr := bridge.ResolveTextToNodeID(tCtx, sel.Value)
				cancel()
				if resolveErr != nil {
					results = append(results, actionResult{
						Index: i, Success: false,
						Error: fmt.Sprintf("text selector: %v", resolveErr),
					})
					if req.StopOnError {
						break
					}
					continue
				}
				step.NodeID = nid
				step.Selector = ""
				step.Ref = ""
			case selector.KindSemantic:
				if h.Matcher != nil {
					tCtx, cancel := context.WithTimeout(ctx, stepTimeout)
					nodes := h.resolveSnapshotNodes(resolvedTabID)
					if len(nodes) == 0 {
						h.refreshRefCache(tCtx, resolvedTabID)
						nodes = h.resolveSnapshotNodes(resolvedTabID)
					}
					if len(nodes) > 0 {
						descs := make([]semantic.ElementDescriptor, len(nodes))
						for j, n := range nodes {
							descs[j] = semantic.ElementDescriptor{
								Ref: n.Ref, Role: n.Role, Name: n.Name, Value: n.Value,
							}
						}
						findResult, findErr := h.Matcher.Find(tCtx, sel.Value, descs, semantic.FindOptions{
							Threshold: 0.3, TopK: 1,
						})
						if findErr == nil && findResult.BestRef != "" {
							step.Ref = findResult.BestRef
							cache := h.Bridge.GetRefCache(resolvedTabID)
							if cache != nil {
								if nid, ok := cache.Refs[findResult.BestRef]; ok {
									step.NodeID = nid
								}
							}
						}
					}
					cancel()
					if step.NodeID == 0 {
						results = append(results, actionResult{
							Index: i, Success: false,
							Error: fmt.Sprintf("semantic selector %q: no matching element found", sel.Value),
						})
						if req.StopOnError {
							break
						}
						continue
					}
				} else {
					results = append(results, actionResult{
						Index: i, Success: false,
						Error: "semantic selectors require a matcher (not configured)",
					})
					if req.StopOnError {
						break
					}
					continue
				}
				step.Selector = ""
			}
		}

		// Cache intent before execution so recovery can reconstruct the query.
		// Only cache when the ref IS in the snapshot to avoid overwriting
		// the richer /find-cached entry (which has the Query).
		if !useLiteAction && step.Ref != "" && h.Recovery != nil && !stepRefMissing {
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
					res, _, err := h.executeAction(ctx, step)
					return res, err
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
			res, _, err = h.executeAction(tCtx, step)
			if err != nil && step.Ref != "" && shouldRetryStaleRef(err) {
				recordStaleRefRetry()
				h.refreshRefCache(tCtx, resolvedTabID)
				if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
					if nid, ok := cache.Refs[step.Ref]; ok {
						step.NodeID = nid
						res, _, err = h.executeAction(tCtx, step)
					}
				}
			}
			// Semantic self-healing for macro steps.
			if err != nil && step.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, step.Ref) {
				rr, recRes, recErr := h.Recovery.AttemptWithClassification(
					tCtx, resolvedTabID, step.Ref, step.Kind,
					recovery.ClassifyFailure(err),
					func(ctx context.Context, kind string, nodeID int64) (map[string]any, error) {
						step.NodeID = nodeID
						res, _, err := h.executeAction(ctx, step)
						return res, err
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

	httpx.JSON(w, 200, map[string]any{
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
	h.Recovery.RecordIntent(tabID, req.Ref, recovery.IntentEntry{
		Descriptor: desc,
		CachedAt:   time.Now(),
	})
}

func (h *Handlers) executeAction(ctx context.Context, req bridge.ActionRequest) (map[string]any, string, error) {
	if h.shouldUseLiteAction(req.Kind) {
		return h.executeLiteAction(ctx, req)
	}

	if err := h.ensureChrome(); err != nil {
		return nil, "", fmt.Errorf("chrome initialization: %w", err)
	}
	result, err := h.Bridge.ExecuteAction(ctx, req.Kind, req)
	return result, "", err
}

func (h *Handlers) shouldUseLiteAction(kind string) bool {
	capability, ok := actionCapability(kind)
	if !ok {
		return h.Router != nil && h.Router.Mode() == engine.ModeLite
	}
	return h.useLite(capability, "")
}

func (h *Handlers) executeLiteAction(ctx context.Context, req bridge.ActionRequest) (map[string]any, string, error) {
	if h.Router == nil || h.Router.Lite() == nil {
		return nil, "", fmt.Errorf("lite engine unavailable")
	}
	switch strings.ToLower(strings.TrimSpace(req.Kind)) {
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
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case bridge.ActionClick:
		return engine.CapClick, true
	case bridge.ActionType, bridge.ActionFill:
		return engine.CapType, true
	default:
		return "", false
	}
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
	h.Bridge.SetRefCache(tabID, &bridge.RefCache{Refs: refs, Nodes: flat})
}
