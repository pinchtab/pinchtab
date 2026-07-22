package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/session"
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

// buildActionRoute assembles the route metadata recorded by the single, batch,
// and macro action endpoints: a single-browser route plus one attempt reflecting
// the handle decision, with the originally requested browser when provided.
func buildActionRoute(resolvedBrowser, requestBrowser string, decision browsers.HandleDecision) *browserops.RouteMetadata {
	route := browserops.SingleBrowserRoute(resolvedBrowser)
	route.Attempts = append(route.Attempts, browserops.RouteAttempt{
		Browser:  resolvedBrowser,
		Accepted: decision.Decision == browsers.DecisionHandle,
		Reason:   decision.Reason,
	})
	if requestBrowser != "" {
		route.RequestedBrowser = requestBrowser
	}
	return route
}

// rejectMixedBrowsers returns the first action's browser as the request browser,
// rejecting (with a 400) any later action that names a different browser, since a
// batch/macro executes on a single browser. noun/field tune the error wording
// ("batch"/"actions" vs "macro"/"steps").
func rejectMixedBrowsers(w http.ResponseWriter, actions []bridge.ActionRequest, noun, field string) (string, bool) {
	var requestBrowser string
	if len(actions) > 0 {
		requestBrowser = strings.TrimSpace(actions[0].Browser)
	}
	for i, a := range actions {
		if b := strings.TrimSpace(a.Browser); b != "" && !strings.EqualFold(b, requestBrowser) {
			httpx.Error(w, 400, fmt.Errorf("mixed browser values in a %s are not supported: %s[0]=%q, %s[%d]=%q", noun, field, requestBrowser, field, i, b))
			return "", false
		}
	}
	return requestBrowser, true
}

func rejectMultiStepSubmitClicks(w http.ResponseWriter, actions []bridge.ActionRequest, noun, field string) bool {
	for i, action := range actions {
		if bridge.IsSubmitClick(action.Kind, action) {
			httpx.ErrorCode(
				w,
				http.StatusBadRequest,
				"click_submit_requires_single_action",
				fmt.Sprintf("%s %s[%d] uses click submit; use a single /action request so its bounded post-state can be reported", noun, field, i),
				false,
				nil,
			)
			return false
		}
	}
	return true
}

// dialogAwareActionError shapes a per-action error message: it surfaces a blocking
// dialog's own message, or a hint when a click timed out with a pending dialog,
// otherwise the caller-supplied fallback.
func (h *Handlers) dialogAwareActionError(err error, kind, tabID, fallback string) string {
	var dialogErr *bridge.ErrDialogBlocking
	if errors.As(err, &dialogErr) {
		return err.Error()
	}
	if isClickTimeoutWithPendingDialog(err, kind, tabID, h.Bridge) {
		dm := h.Bridge.GetDialogManager()
		if ds := dm.GetPending(tabID); ds != nil {
			return fmt.Sprintf("action %s timed out; a JavaScript dialog is blocking (%s: %q) — use --dialog-action accept|dismiss",
				kind, ds.Type, ds.Message)
		}
	}
	return fallback
}

// runResolvedActionStep executes one already-resolved action (selector resolution,
// kind check, intent caching, and the refMissing/Recovery guard stay with the
// caller) and shapes the result. On success it follows an auto-switched tab,
// returning the possibly-updated ctx and tab id so the batch/macro loop carries
// them forward. The caller owns the tCtx lifetime.
func (h *Handlers) runResolvedActionStep(
	ctx, tCtx context.Context,
	r *http.Request,
	w http.ResponseWriter,
	step *bridge.ActionRequest,
	cfg *config.RuntimeConfig,
	tabID string,
	index int,
	refMissing bool,
	errFallback func(error) string,
) (actionResult, context.Context, string) {
	res, _, _, err := h.executeActionResilient(tCtx, step, cfg, tabID, refMissing)
	nextCtx := ctx
	nextTabID := tabID
	if err == nil {
		if switched := switchedTabFromActionResult(res); switched != "" {
			switchedCtx, switchedTabID, switchErr := h.tabContext(r, switched)
			if switchErr != nil {
				err = fmt.Errorf("auto-switch tab %s: %w", switched, switchErr)
			} else {
				nextCtx = switchedCtx
				nextTabID = switchedTabID
				markCreatedTab(w, nextTabID)
				h.recordResolvedTab(r, nextTabID)
			}
		}
	}
	if err != nil {
		return actionResult{
			Index:   index,
			Success: false,
			Error:   h.dialogAwareActionError(err, step.Kind, nextTabID, errFallback(err)),
		}, nextCtx, nextTabID
	}
	return actionResult{Index: index, Success: true, Result: res}, nextCtx, nextTabID
}

func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	var req bridge.ActionRequest
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		d := newQueryDecoder(q)
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
		d.Int64("nodeId", &req.NodeID)
		if d.present("x") {
			d.Float("x", &req.X)
			req.HasXY = true
		}
		if d.present("y") {
			d.Float("y", &req.Y)
			req.HasXY = true
		}
		var hasXYParam bool
		d.Bool("hasXY", &hasXYParam)
		req.HasXY = req.HasXY || hasXYParam
		req.Button = q.Get("button")
		d.Bool("dismissBanners", &req.DismissBanners)
		d.Bool("dismissKnownInterstitials", &req.DismissKnownInterstitials)
		d.Bool("submit", &req.Submit)
		if d.present("autoSwitch") {
			var autoSwitch bool
			d.Bool("autoSwitch", &autoSwitch)
			req.AutoSwitch = &autoSwitch
		}
		d.Int("deltaX", &req.DeltaX)
		d.Int("deltaY", &req.DeltaY)
		req.Browser = q.Get("browser")
		if err := d.Err(); err != nil {
			httpx.Error(w, 400, err)
			return
		}
	} else {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
		req.Kind = bridge.CanonicalActionKind(req.Kind)
		req.DialogAction = strings.ToLower(strings.TrimSpace(req.DialogAction))
	}

	routing, ok := h.resolveBrowserForRequest(w, r, req.TabID, strings.TrimSpace(req.Browser), browsers.RequestIntent{
		Shape:         browsers.ShapeInteraction,
		StateChanging: true,
	})
	if !ok {
		return
	}
	requestBrowser := routing.RequestBrowser
	resolvedBrowser := routing.Browser
	handleDecision := routing.Decision
	effectiveCfg := routing.EffectiveCfg

	req.Browser = resolvedBrowser

	// Single endpoint returns 400 for bad input, unlike batch which returns 200 with per-action errors.
	if req.Kind == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'kind'"))
		return
	}
	if req.DialogAction != "" && req.DialogAction != "accept" && req.DialogAction != "dismiss" {
		httpx.Error(w, 400, fmt.Errorf("dialogAction must be 'accept' or 'dismiss'"))
		return
	}
	if err := bridge.ValidateSubmitAction(req.Kind, req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_submit_action", err.Error(), false, nil)
		return
	}
	h.recordActionRequest(r, req)
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

	var resolvedTabID string
	var ctx context.Context
	{
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

	actionTimeout := effectiveCfg.ActionTimeout
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

	if req.DismissKnownInterstitials {
		if _, err := h.dismissKnownInterstitials(tCtx, resolvedTabID); err != nil {
			httpx.ErrorCode(w, http.StatusConflict, "known_interstitial_not_dismissed", err.Error(), true, nil)
			return
		}
	}

	selectorResolution, err := h.resolveActionRequestSelector(tCtx, resolvedTabID, &req)
	if err != nil {
		httpx.Error(w, selectorResolution.httpStatus(), err)
		return
	}
	refMissing := selectorResolution.refMissing
	submitClick := bridge.IsSubmitClick(req.Kind, req)
	if submitClick && refMissing {
		httpx.ErrorCode(w, http.StatusNotFound, "submit_target_not_found", fmt.Sprintf("ref %s not found - take a /snapshot first", req.Ref), false, map[string]any{
			"dispatched": false,
		})
		return
	}
	if submitClick && req.NodeID <= 0 {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_submit_target", "click submit requires a selector, ref, or nodeId", false, map[string]any{
			"dispatched": false,
		})
		return
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
	if refMissing && (req.Ref == "" || h.Recovery == nil) {
		httpx.Error(w, 404, fmt.Errorf("ref %s not found - take a /snapshot first", req.Ref))
		return
	}

	var submitBefore submitStateSnapshot
	if submitClick {
		submitBefore, err = h.captureSubmitState(tCtx, resolvedTabID)
		if err != nil {
			httpx.ErrorCode(w, http.StatusInternalServerError, "submit_pre_state_failed", fmt.Sprintf("capture pre-submit state: %v", err), true, map[string]any{
				"dispatched": false,
			})
			return
		}
	}

	result, actionBackend, recoveryResult, actionErr := h.executeActionResilient(tCtx, &req, effectiveCfg, resolvedTabID, refMissing)
	submitTimeoutWithDialog := submitClick && isClickTimeoutWithPendingDialog(actionErr, req.Kind, resolvedTabID, h.Bridge)
	if submitClick && !submitTimeoutWithDialog && (actionErr == nil || errors.Is(actionErr, context.DeadlineExceeded)) {
		actionTimedOut := errors.Is(actionErr, context.DeadlineExceeded)
		dispatch := "acknowledged"
		if actionTimedOut {
			dispatch = "unconfirmed"
		}

		// tCtx may be expired here. Keep the live tab context as the parent and
		// give post-state observation its own bounded, client-cancelable child.
		postCtx, postCancel := context.WithTimeout(ctx, postSubmitPollTimeout)
		go httpx.CancelOnClientDone(r.Context(), postCancel)
		postState, postErr := h.pollSubmitPostState(postCtx, resolvedTabID, submitBefore, dispatch, actionTimedOut)
		postCancel()
		if postErr != nil {
			httpx.ErrorCode(w, http.StatusInternalServerError, "submit_post_state_failed", postErr.Error(), false, map[string]any{
				"dispatch":       dispatch,
				"actionTimedOut": actionTimedOut,
				"doNotRetry":     true,
			})
			return
		}
		if result == nil {
			result = make(map[string]any)
		}
		result["postState"] = postState
		actionErr = nil
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
		if browserops.IsIDPIBlocked(actionErr) {
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
		retryable := !submitClick
		var details map[string]any
		if submitClick {
			details = map[string]any{
				"dispatch":   "unconfirmed",
				"doNotRetry": true,
			}
		}
		httpx.ErrorCode(w, 500, "action_failed", fmt.Sprintf("action %s: %v", req.Kind, actionErr), retryable, details)
		return
	}

	if actionBackend == "" {
		actionBackend = "chrome"
	}
	if actionBackend != "static" {
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
		markCreatedTab(w, switched)
		h.recordResolvedTab(r, switched)
	}
	actionRoute := buildActionRoute(resolvedBrowser, requestBrowser, handleDecision)
	h.recordActivity(r, activity.Update{Route: actionRoute})
	resp := map[string]any{"success": true, "result": result, "route": actionRoute}
	if recoveryResult != nil {
		resp["recovery"] = recoveryResult
	}
	httpx.JSON(w, 200, resp)
}

// @Endpoint POST /tabs/{id}/action
func (h *Handlers) HandleTabAction(w http.ResponseWriter, r *http.Request) {
	h.withPathTabIDBody(w, r, h.HandleAction)
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
	if !rejectMultiStepSubmitClicks(w, req.Actions, "batch", "actions") {
		return
	}

	h.handleActionsBatch(w, r, req)
}

// @Endpoint POST /tabs/{id}/actions
func (h *Handlers) HandleTabActions(w http.ResponseWriter, r *http.Request) {
	h.withPathTabIDBody(w, r, h.HandleActions)
}

func (h *Handlers) handleActionsBatch(w http.ResponseWriter, r *http.Request, req actionsRequest) {
	// Browser resolution: use the first action's browser field as the request
	// browser, then fall through session > instance > global default > chrome.
	requestBrowser, ok := rejectMixedBrowsers(w, req.Actions, "batch", "actions")
	if !ok {
		return
	}
	routing, ok := h.resolveBrowserForRequest(w, r, req.TabID, requestBrowser, browsers.RequestIntent{
		Shape:         browsers.ShapeInteraction,
		StateChanging: true,
	})
	if !ok {
		return
	}
	resolvedBrowser := routing.Browser
	handleDecision := routing.Decision
	effectiveCfg := routing.EffectiveCfg

	var ctx context.Context
	var resolvedTabID string
	owner := resolveOwner(r, req.Owner)
	{
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
		} else if action.TabID != resolvedTabID {
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

		tCtx, tCancel := context.WithTimeout(ctx, effectiveCfg.ActionTimeout)

		selectorResolution, resolveErr := h.resolveActionRequestSelector(tCtx, resolvedTabID, &action)
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

		var stop bool
		ctx, resolvedTabID, stop = h.runMultiStepActionTail(ctx, tCtx, tCancel, r, w, &action, effectiveCfg, resolvedTabID, i, refMissing, req.StopOnError, func(err error) string {
			return fmt.Sprintf("action %s: %v", action.Kind, err)
		}, &results)
		if stop {
			break
		}
	}

	batchRoute := buildActionRoute(resolvedBrowser, requestBrowser, handleDecision)
	h.writeMultiStepActionResult(w, r, ctx, resolvedTabID, results, len(req.Actions), batchRoute, nil)
}

// runMultiStepActionTail runs the per-step work shared by the /actions batch and
// /macro loops, once each surface's divergent pre-step work (tab switch, timeout
// model, selector resolution, kind validation) is done and refMissing is known:
// it caches action intent, rejects a missing ref when no recovery is configured,
// runs the resolved action step under tCtx, and appends the result. It always
// releases tCtx via cancel before returning. errFmt formats a step failure
// message per-surface. It returns the (possibly auto-switch-updated) ctx +
// resolvedTabID and whether the loop should stop (StopOnError on a failure).
func (h *Handlers) runMultiStepActionTail(
	ctx, tCtx context.Context, cancel context.CancelFunc,
	r *http.Request, w http.ResponseWriter,
	step *bridge.ActionRequest, cfg *config.RuntimeConfig,
	resolvedTabID string, index int, refMissing, stopOnError bool,
	errFmt func(error) string,
	results *[]actionResult,
) (context.Context, string, bool) {
	// Cache intent before execution so recovery can reconstruct the query.
	// Only cache when the ref IS in the snapshot to avoid overwriting the
	// richer /find-cached entry (which has the Query).
	if step.Ref != "" && h.Recovery != nil && !refMissing {
		h.cacheActionIntent(resolvedTabID, *step)
	}

	if refMissing && h.Recovery == nil {
		cancel()
		*results = append(*results, actionResult{
			Index: index, Success: false,
			Error: fmt.Sprintf("ref %s not found - take a /snapshot first", step.Ref),
		})
		return ctx, resolvedTabID, stopOnError
	}

	var result actionResult
	result, ctx, resolvedTabID = h.runResolvedActionStep(ctx, tCtx, r, w, step, cfg, resolvedTabID, index, refMissing, errFmt)
	cancel()
	*results = append(*results, result)
	return ctx, resolvedTabID, !result.Success && stopOnError
}

// writeMultiStepActionResult finalizes a multi-step run shared by the /actions
// batch and /macro surfaces: it counts successes, fires the auto-solver when any
// step succeeded, records the route activity, and writes the 200 JSON response.
// total is the requested step count (not len(results), which is shorter when
// StopOnError fired). extra carries surface-specific top-level keys (e.g. macro's
// "kind") merged into the shared {results,total,successful,failed,route} shape.
func (h *Handlers) writeMultiStepActionResult(
	w http.ResponseWriter, r *http.Request,
	ctx context.Context, resolvedTabID string,
	results []actionResult, total int,
	route *browserops.RouteMetadata, extra map[string]any,
) {
	successful := countSuccessful(results)
	if successful > 0 {
		h.maybeAutoSolve(ctx, resolvedTabID, autoSolverTriggerAction)
	}
	h.recordActivity(r, activity.Update{Route: route})
	resp := map[string]any{
		"results":    results,
		"total":      total,
		"successful": successful,
		"failed":     total - successful,
		"route":      route,
	}
	for k, v := range extra {
		resp[k] = v
	}
	httpx.JSON(w, 200, resp)
}

func (h *Handlers) HandleMacro(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowMacro {
		h.writeCapabilityDisabled(w, routes.CapMacro)
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
	if !rejectMultiStepSubmitClicks(w, req.Steps, "macro", "steps") {
		return
	}
	owner := resolveOwner(r, req.Owner)

	// Browser resolution: use the first step's browser field as the request
	// browser, then fall through session > instance > global default > chrome.
	macroRequestBrowser, ok := rejectMixedBrowsers(w, req.Steps, "macro", "steps")
	if !ok {
		return
	}
	var macroSessionBrowser string
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		macroSessionBrowser = sess.Browser
	}
	var macroInstanceBrowser string
	if req.TabID != "" && h.Orchestrator != nil {
		if inst, ok := h.Orchestrator.FindInstanceByTab(req.TabID); ok && inst != nil && inst.Browser != "" {
			macroInstanceBrowser = inst.Browser
		}
	}
	macroResolvedBrowser := config.ResolveBrowser(macroRequestBrowser, macroSessionBrowser, macroInstanceBrowser, h.Config.DefaultBrowser, h.Config.BrowsersAvailable)
	if macroResolvedBrowser != config.BrowserChrome {
		if _, err := config.ParseBrowser(macroResolvedBrowser, h.Config.BrowsersAvailable); err != nil {
			httpx.Error(w, http.StatusBadRequest, err)
			return
		}
	}
	macroHandleDecision, err := checkBrowserCanHandle(macroResolvedBrowser, browsers.RequestIntent{
		Shape:         browsers.ShapeInteraction,
		StateChanging: true,
	})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return
	}
	if macroHandleDecision.Decision == browsers.DecisionSkip {
		macroResolvedBrowser = config.BrowserChrome
	}

	macroEffectiveCfg, err := h.resolveEffectiveConfig(macroResolvedBrowser)
	if err != nil {
		var ambErr *config.AmbiguousBrowserError
		if errors.As(err, &ambErr) {
			httpx.ErrorCode(w, http.StatusBadRequest, "browser_ambiguous", err.Error(), false, map[string]any{
				"browser": ambErr.Browser,
				"targets": ambErr.Targets,
			})
		} else {
			httpx.Error(w, http.StatusBadRequest, err)
		}
		return
	}

	stepTimeout := macroEffectiveCfg.ActionTimeout
	if req.StepTimeout > 0 && req.StepTimeout <= 60 {
		stepTimeout = time.Duration(req.StepTimeout * float64(time.Second))
	}

	var ctx context.Context
	var resolvedTabID string
	{
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
		selectorCtx, selectorCancel := context.WithTimeout(ctx, stepTimeout)
		selectorResolution, resolveErr := h.resolveActionRequestSelector(selectorCtx, resolvedTabID, &step)
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

		tCtx, cancel := context.WithTimeout(ctx, stepTimeout)
		var stop bool
		ctx, resolvedTabID, stop = h.runMultiStepActionTail(ctx, tCtx, cancel, r, w, &step, macroEffectiveCfg, resolvedTabID, i, stepRefMissing, req.StopOnError, func(err error) string {
			return err.Error()
		}, &results)
		if stop {
			break
		}
	}

	macroRoute := buildActionRoute(macroResolvedBrowser, macroRequestBrowser, macroHandleDecision)
	h.writeMultiStepActionResult(w, r, ctx, resolvedTabID, results, len(req.Steps), macroRoute, map[string]any{"kind": "macro"})
}
