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

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/httpx"
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

func (h *Handlers) enforceTabNotPausedForHandoff(tabID string) error {
	if tabID == "" {
		return nil
	}
	ctrl, ok := h.handoffController()
	if !ok {
		return nil
	}
	state, exists := ctrl.TabHandoffState(tabID)
	if !exists || state.Status != "paused_handoff" {
		return nil
	}
	if state.Reason != "" {
		return fmt.Errorf("tab %s is paused for human handoff (%s)", tabID, state.Reason)
	}
	return fmt.Errorf("tab %s is paused for human handoff", tabID)
}

// HandleAction performs a single action on a tab (click, type, fill, etc).
func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	var req bridge.ActionRequest
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.Kind = bridge.CanonicalActionKind(q.Get("kind"))
		req.TabID = q.Get("tabId")
		req.Owner = q.Get("owner")
		req.Ref = q.Get("ref")
		req.Selector = q.Get("selector")
		req.Text = q.Get("text")
		req.Value = q.Get("value")
		req.Key = q.Get("key")
		req.DialogAction = strings.ToLower(strings.TrimSpace(q.Get("dialogAction")))
		req.DialogText = q.Get("dialogText")
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
				req.HasXY = req.HasXY || b
			}
		}
		req.Button = q.Get("button")
		if v := q.Get("dismissBanners"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				req.DismissBanners = b
			}
		}
		if v := q.Get("autoSwitch"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				req.AutoSwitch = &b
			}
		}
		if vals, ok := q["deltaX"]; ok && len(vals) > 0 {
			if n, err := strconv.Atoi(vals[0]); err == nil {
				req.DeltaX = n
			}
		}
		if vals, ok := q["deltaY"]; ok && len(vals) > 0 {
			if n, err := strconv.Atoi(vals[0]); err == nil {
				req.DeltaY = n
			}
		}
	} else {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
		req.Kind = bridge.CanonicalActionKind(req.Kind)
		req.DialogAction = strings.ToLower(strings.TrimSpace(req.DialogAction))
	}

	// Validate kind — single endpoint returns 400 for bad input (unlike batch which returns 200 with errors)
	if req.Kind == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'kind'"))
		return
	}
	if req.DialogAction != "" && req.DialogAction != "accept" && req.DialogAction != "dismiss" {
		httpx.Error(w, 400, fmt.Errorf("dialogAction must be 'accept' or 'dismiss'"))
		return
	}
	h.recordActionRequest(r, req)
	if !h.shouldUseLiteAction(req) {
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
	useLiteAction := h.shouldUseLiteAction(req)
	var resolvedTabID string
	var ctx context.Context
	if useLiteAction {
		ctx = r.Context()
		resolvedTabID = req.TabID
	} else {
		var err error
		ctx, resolvedTabID, err = h.tabContext(r, req.TabID)
		if err != nil {
			WriteTabContextError(w, err, 404)
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
		if err := h.enforceTabNotPausedForHandoff(resolvedTabID); err != nil {
			httpx.ErrorCode(w, 409, "tab_paused_handoff", err.Error(), false, h.handoffErrorDetails(resolvedTabID))
			return
		}
		defer h.armAutoCloseIfEnabled(resolvedTabID)
	}
	h.recordResolvedTab(r, resolvedTabID)
	w.Header().Set(activity.HeaderPTTabID, resolvedTabID)

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
	// and resolve browser/semantic selectors to node IDs when possible.
	selectorResolution, err := h.resolveActionRequestSelector(tCtx, resolvedTabID, useLiteAction, &req)
	if err != nil {
		httpx.Error(w, selectorResolution.httpStatus(), err)
		return
	}
	refMissing := selectorResolution.refMissing

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
		if actionErr != nil && shouldRetryPointerAction(req, actionErr) {
			if req.Ref != "" && shouldRetryStaleRef(actionErr) {
				recordStaleRefRetry()
				h.refreshRefCache(tCtx, resolvedTabID)
				if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
					if target, ok := cache.Lookup(req.Ref); ok {
						req.NodeID = target.BackendNodeID
					}
				}
			}
			h.refreshActionNodeIDFromSelector(tCtx, &req)
			time.Sleep(pointerRetryDelay)
			result, engineName, actionErr = h.executeAction(tCtx, req)
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
		if engine.IsIDPIBlocked(actionErr) {
			httpx.ErrorCode(w, http.StatusForbidden, "idpi_blocked", actionErr.Error(), false, nil)
			return
		}
		var dialogErr *bridge.ErrDialogBlocking
		if errors.As(actionErr, &dialogErr) {
			httpx.ErrorCode(w, 500, "dialog_blocking", actionErr.Error(), false, map[string]any{
				"suggestion":     "use --dialog-action accept or --dialog-action dismiss",
				"dialog_type":    dialogErr.DialogType,
				"dialog_message": dialogErr.DialogMessage,
			})
			return
		}
		if isClickTimeoutWithPendingDialog(actionErr, req.Kind, resolvedTabID, h.Bridge) {
			dm := h.Bridge.GetDialogManager()
			dialogState := dm.GetPending(resolvedTabID)
			msg := fmt.Sprintf("action %s timed out; a JavaScript dialog is blocking (%s: %q)",
				req.Kind, dialogState.Type, dialogState.Message)
			httpx.ErrorCode(w, 500, "dialog_blocking", msg, false, map[string]any{
				"suggestion":     "use --dialog-action accept or --dialog-action dismiss",
				"dialog_type":    dialogState.Type,
				"dialog_message": dialogState.Message,
			})
			return
		}
		httpx.ErrorCode(w, 500, "action_failed", fmt.Sprintf("action %s: %v", req.Kind, actionErr), true, nil)
		return
	}

	if engineName == "" {
		engineName = "chrome"
	}
	if engineName != "lite" {
		h.maybeAutoSolve(tCtx, resolvedTabID, autoSolverTriggerAction)
		// Banner dismissal only makes sense when the click triggered a
		// navigation (waitNav settles us on a fresh page). Without waitNav we
		// skip — the caller is interacting within the current document and
		// any banner has either been dismissed already or is irrelevant.
		if req.WaitNav && req.DismissBanners {
			h.dismissBanners(tCtx, resolvedTabID, true)
		}
	}
	// If the click opened (and auto-switched to) a new tab, point the
	// request-scoped current tab at it so the next action lands there.
	if switched := switchedTabFromActionResult(result); switched != "" {
		h.setCurrentTabForRequest(r, switched)
		w.Header().Set(activity.HeaderPTTabID, switched)
		h.recordResolvedTab(r, switched)
	}
	w.Header().Set("X-Engine", engineName)
	h.recordEngine(r, engineName)
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

	// Use lite tab resolution only when every action can stay on the lite path.
	allLite := h.Router != nil && h.Router.Mode() == engine.ModeLite
	if allLite {
		for _, action := range req.Actions {
			if !h.shouldUseLiteAction(action) {
				allLite = false
				break
			}
		}
	}
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
			WriteTabContextError(w, err, 404)
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
			if err := h.enforceTabNotPausedForHandoff(resolvedTabID); err != nil {
				results = append(results, actionResult{Index: i, Success: false, Error: err.Error()})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
		useLiteAction := h.shouldUseLiteAction(action)

		selectorResolution, resolveErr := h.resolveActionRequestSelector(tCtx, resolvedTabID, useLiteAction, &action)
		if resolveErr != nil {
			tCancel()
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: resolveErr.Error(),
			})
			if req.StopOnError {
				break
			}
			continue
		}
		refMissing := selectorResolution.refMissing

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
			if err != nil && shouldRetryPointerAction(action, err) {
				if action.Ref != "" && shouldRetryStaleRef(err) {
					recordStaleRefRetry()
					h.refreshRefCache(tCtx, resolvedTabID)
					if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
						if target, ok := cache.Lookup(action.Ref); ok {
							action.NodeID = target.BackendNodeID
						}
					}
				}
				h.refreshActionNodeIDFromSelector(tCtx, &action)
				time.Sleep(pointerRetryDelay)
				actionRes, _, err = h.executeAction(tCtx, action)
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
		if err == nil && !allLite {
			if switched := switchedTabFromActionResult(actionRes); switched != "" {
				nextCtx, nextTabID, switchErr := h.tabContext(r, switched)
				if switchErr != nil {
					err = fmt.Errorf("auto-switch tab %s: %w", switched, switchErr)
				} else {
					ctx = nextCtx
					resolvedTabID = nextTabID
					w.Header().Set(activity.HeaderPTTabID, nextTabID)
					h.recordResolvedTab(r, nextTabID)
				}
			}
		}

		if err != nil {
			errMsg := fmt.Sprintf("action %s: %v", action.Kind, err)
			var dialogErr *bridge.ErrDialogBlocking
			if errors.As(err, &dialogErr) {
				errMsg = err.Error()
			} else if isClickTimeoutWithPendingDialog(err, action.Kind, resolvedTabID, h.Bridge) {
				dm := h.Bridge.GetDialogManager()
				if ds := dm.GetPending(resolvedTabID); ds != nil {
					errMsg = fmt.Sprintf("action %s timed out; a JavaScript dialog is blocking (%s: %q) — use --dialog-action accept|dismiss",
						action.Kind, ds.Type, ds.Message)
				}
			}
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: errMsg,
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

	successful := countSuccessful(results)
	if !allLite && successful > 0 {
		h.maybeAutoSolve(ctx, resolvedTabID, autoSolverTriggerAction)
	}

	httpx.JSON(w, 200, map[string]any{
		"results":    results,
		"total":      len(req.Actions),
		"successful": successful,
		"failed":     len(req.Actions) - successful,
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
	if allLiteMacro {
		for _, step := range req.Steps {
			if !h.shouldUseLiteAction(step) {
				allLiteMacro = false
				break
			}
		}
	}
	var ctx context.Context
	var resolvedTabID string
	if allLiteMacro {
		ctx = r.Context()
		resolvedTabID = req.TabID
	} else {
		var err error
		ctx, resolvedTabID, err = h.tabContext(r, req.TabID)
		if err != nil {
			WriteTabContextError(w, err, 404)
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
			if err := h.enforceTabNotPausedForHandoff(resolvedTabID); err != nil {
				results = append(results, actionResult{Index: i, Success: false, Error: err.Error()})
				if req.StopOnError {
					break
				}
				continue
			}
		}
		useLiteAction := h.shouldUseLiteAction(step)
		selectorCtx, selectorCancel := context.WithTimeout(ctx, stepTimeout)
		selectorResolution, resolveErr := h.resolveActionRequestSelector(selectorCtx, resolvedTabID, useLiteAction, &step)
		selectorCancel()
		if resolveErr != nil {
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: resolveErr.Error(),
			})
			if req.StopOnError {
				break
			}
			continue
		}
		stepRefMissing := selectorResolution.refMissing

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
			if err != nil && shouldRetryPointerAction(step, err) {
				if step.Ref != "" && shouldRetryStaleRef(err) {
					recordStaleRefRetry()
					h.refreshRefCache(tCtx, resolvedTabID)
					if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
						if target, ok := cache.Lookup(step.Ref); ok {
							step.NodeID = target.BackendNodeID
						}
					}
				}
				h.refreshActionNodeIDFromSelector(tCtx, &step)
				time.Sleep(pointerRetryDelay)
				res, _, err = h.executeAction(tCtx, step)
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
		if err == nil && !allLiteMacro {
			if switched := switchedTabFromActionResult(res); switched != "" {
				nextCtx, nextTabID, switchErr := h.tabContext(r, switched)
				if switchErr != nil {
					err = fmt.Errorf("auto-switch tab %s: %w", switched, switchErr)
				} else {
					ctx = nextCtx
					resolvedTabID = nextTabID
					w.Header().Set(activity.HeaderPTTabID, nextTabID)
					h.recordResolvedTab(r, nextTabID)
				}
			}
		}
		if err != nil {
			errMsg := err.Error()
			var dialogErr *bridge.ErrDialogBlocking
			if errors.As(err, &dialogErr) {
				// Error message is already formatted by ErrDialogBlocking.Error()
			} else if isClickTimeoutWithPendingDialog(err, step.Kind, resolvedTabID, h.Bridge) {
				dm := h.Bridge.GetDialogManager()
				if ds := dm.GetPending(resolvedTabID); ds != nil {
					errMsg = fmt.Sprintf("action %s timed out; a JavaScript dialog is blocking (%s: %q) — use --dialog-action accept|dismiss",
						step.Kind, ds.Type, ds.Message)
				}
			}
			results = append(results, actionResult{Index: i, Success: false, Error: errMsg})
			if req.StopOnError {
				break
			}
			continue
		}
		results = append(results, actionResult{Index: i, Success: true, Result: res})
	}

	successful := countSuccessful(results)
	if !allLiteMacro && successful > 0 {
		h.maybeAutoSolve(ctx, resolvedTabID, autoSolverTriggerAction)
	}

	httpx.JSON(w, 200, map[string]any{
		"kind":       "macro",
		"results":    results,
		"total":      len(req.Steps),
		"successful": successful,
		"failed":     len(req.Steps) - successful,
	})
}
