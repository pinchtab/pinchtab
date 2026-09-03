package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/pinchtab/internal/session"
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

// A stale ref whose target submits a form is refused, and the only correct next move is to
// re-read the page and click the ref that is actually there. `--submit` is the wrong advice
// from this state — it answers 404 from the same staleness — so the remedy is the snapshot.
const staleSubmitTargetHint = "The ref came from an older snapshot and now resolves to a control that submits a form. Nothing was dispatched. Take a fresh snapshot and click the ref it reports; do not retry this ref, and do not add --submit, which cannot resolve a stale ref either."

var reSnapshot = remedy.Declare("pinchtab snap")

func staleSubmitTargetDetails() map[string]any {
	details := remedy.Details(staleSubmitTargetHint, reSnapshot.Remedy())
	// The submit family reports dispatch state on every refusal, and this one is the
	// reason the card exists: nothing was clicked.
	details["dispatched"] = false
	return details
}

// writeTargetNotFound is the one response for a target that cannot be resolved, whichever
// path exhausted it. 404 rather than 500 because the request named something that is not
// there; retryable is absent because an absent target stays absent. The recovery record
// carries the matcher's score and threshold when there is one — diagnosis belongs in
// details, never in the sentence a caller reads as the reason.
func writeTargetNotFound(w http.ResponseWriter, err error, rr *recovery.RecoveryResult) {
	details := map[string]any{"dispatched": false}
	if rr != nil {
		details["recovery"] = rr
	}
	httpx.ErrorCode(w, http.StatusNotFound, "ref_not_found", err.Error(), false, details)
}

// actionFailureIsRetryable answers the only question the flag promises: could repeating the
// IDENTICAL request plausibly succeed. It used to be !submitClick, which says whether the
// caller declared a submit and nothing about the failure, so every unresolvable ref and
// every unsatisfiable body was advertised as worth retrying. A permanently unsatisfiable
// failure never is, and a dispatch that may already have landed must not be repeated
// whatever the error was.
func actionFailureIsRetryable(err error, dispatchMayHaveLanded bool) bool {
	if err == nil || dispatchMayHaveLanded {
		return false
	}
	return !errors.Is(err, ErrTargetNotFound) && !errors.Is(err, bridge.ErrInvalidActionRequest)
}

func (h *Handlers) mapDialogBlockingError(err error, kind, tabID string) (string, *bridge.DialogState, bool) {
	var dialogErr *bridge.ErrDialogBlocking
	if errors.As(err, &dialogErr) {
		return err.Error(), &bridge.DialogState{Type: dialogErr.DialogType, Message: dialogErr.DialogMessage}, true
	}
	if isTimeoutWithPendingDialog(err, tabID, h.Bridge) {
		dialog := pendingTabDialog(h.Bridge, tabID)
		return fmt.Sprintf("action %s timed out; a JavaScript dialog is blocking (%s: %q)", kind, dialog.Type, dialog.Message), dialog, true
	}
	return "", nil, false
}

func (h *Handlers) dialogAwareActionError(err error, kind, tabID, fallback string) string {
	if message, _, ok := h.mapDialogBlockingError(err, kind, tabID); ok {
		return message
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

// queryUndecodedActionFields maps each bridge.ActionRequest JSON key the GET form does not
// decode to why. It is the OWNER of that fact: the GET branch refuses a request carrying one
// of these, and the query/body parity guard excuses exactly these from comparison. A shift
// click sent as ?modifiers=8 used to answer 200 for a plain click, so an undecoded field
// recorded here but not enforced is a wrong action reported as success.
var queryUndecodedActionFields = map[string]string{
	"hasText":   "derived: the flag comes from the presence of text/value, so it is not a parameter of its own",
	"hasToXY":   "derived: the flag comes from the presence of toX/toY",
	"mode":      "the GET form cannot express it; a caller needing it must POST",
	"frameW":    "the GET form cannot express it; a caller needing it must POST",
	"frameH":    "the GET form cannot express it; a caller needing it must POST",
	"modifiers": "the GET form cannot express it; a caller needing a key chord must POST",
	"dragX":     "the GET form cannot express it; a caller needing a drag must POST",
	"dragY":     "the GET form cannot express it; a caller needing a drag must POST",
	"toNodeId":  "the GET form cannot express it; a caller needing a drag destination by node must POST",
	"waitNav":   "the GET form cannot express it; a caller needing to wait for navigation must POST",
	"fast":      "the GET form cannot express it; a caller needing it must POST",
	"humanize":  "the GET form cannot express it; a caller needing humanized input must POST",
}

// unsupportedQueryFields names every recorded field the query supplies, sorted, so the
// refusal is the same message on every run rather than whichever key the map yielded first.
// Presence follows the decoder's own rule — a non-empty value — so ?humanize= is absent
// rather than an unsupported request.
func unsupportedQueryFields(q url.Values) []string {
	var offenders []string
	for key := range queryUndecodedActionFields {
		if strings.TrimSpace(q.Get(key)) != "" {
			offenders = append(offenders, key)
		}
	}
	sort.Strings(offenders)
	return offenders
}

// getOnlyActionQueryKeys are the parameters HandleAction reads from the query ITSELF, which
// bridge.ActionRequest therefore does not declare. They exist because the GET form has no JSON
// body to carry them, so deriving the allow-list from the request type alone refuses a
// parameter this handler implements — the refusal would name the key as "not a parameter of
// /action" while the code reading it sits in the same function.
//
// Each entry records why it is GET-only. TestEveryQueryParameterActionReadsIsAllowed is the
// guard that keeps this in step: it walks every r.URL.Query().Get literal in actions.go and
// requires the key to be allowed, so the next GET-only parameter cannot be silently refused.
var getOnlyActionQueryKeys = map[string]string{
	"timeout": "per-request action timeout in seconds; the GET form has no body to carry it, so HandleAction reads it from the query",
}

// actionQueryKeys is the complete set of meaningful /action query parameters, derived from its
// TWO owners rather than listed by hand: every field bridge.ActionRequest declares, plus what
// the handler reads from the query itself. A field added to either is accepted with no edit
// here, and anything else is a typo or a stray parameter the GET form would drop without a word.
var actionQueryKeys = actionRequestJSONKeys()

func actionRequestJSONKeys() map[string]struct{} {
	keys := make(map[string]struct{})
	for _, field := range reflect.VisibleFields(reflect.TypeOf(bridge.ActionRequest{})) {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys[name] = struct{}{}
		}
	}
	for key := range getOnlyActionQueryKeys {
		keys[key] = struct{}{}
	}
	return keys
}

// unknownQueryFields names every supplied parameter the request type does not declare, sorted
// so the refusal reads the same on every run. Presence follows the decoder's own rule — a
// non-empty value — so ?_= is absent rather than an unknown request.
func unknownQueryFields(q url.Values) []string {
	var unknown []string
	for key := range q {
		if _, known := actionQueryKeys[key]; known {
			continue
		}
		if strings.TrimSpace(q.Get(key)) == "" {
			continue
		}
		unknown = append(unknown, key)
	}
	sort.Strings(unknown)
	return unknown
}

func unknownQueryFieldsError(unknown []string) error {
	named := make([]string, 0, len(unknown))
	for _, key := range unknown {
		if near := nearestActionQueryKey(key); near != "" {
			named = append(named, fmt.Sprintf("%s (did you mean %s?)", key, near))
			continue
		}
		named = append(named, key)
	}
	return fmt.Errorf("%s: not a parameter of /action and would be silently dropped; check the spelling or send the request as POST /action with a JSON body", strings.Join(named, ", "))
}

func nearestActionQueryKey(key string) string {
	if len(key) < 4 {
		return ""
	}
	best := ""
	bestDistance := 0
	for candidate := range actionQueryKeys {
		distance := editDistance(strings.ToLower(key), strings.ToLower(candidate))
		if best == "" || distance < bestDistance || (distance == bestDistance && candidate < best) {
			best, bestDistance = candidate, distance
		}
	}
	if bestDistance > 2 {
		return ""
	}
	return best
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func decodeActionRequest(w http.ResponseWriter, r *http.Request) (bridge.ActionRequest, bool) {
	var req bridge.ActionRequest
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		if offenders := unsupportedQueryFields(q); len(offenders) > 0 {
			httpx.Error(w, 400, fmt.Errorf("%s cannot be sent as query parameters and would be silently dropped; send this as POST /action with a JSON body", strings.Join(offenders, ", ")))
			return bridge.ActionRequest{}, false
		}
		if unknown := unknownQueryFields(q); len(unknown) > 0 {
			httpx.Error(w, 400, unknownQueryFieldsError(unknown))
			return bridge.ActionRequest{}, false
		}
		d := newQueryDecoder(q)
		req.Kind = bridge.CanonicalActionKind(q.Get("kind"))
		req.TabID = q.Get("tabId")
		req.Owner = q.Get("owner")
		req.Ref = q.Get("ref")
		req.Selector = q.Get("selector")
		req.Text = q.Get("text")
		req.Value = q.Get("value")
		req.HasText = d.present("text") || d.present("value")
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
		req.ToSelector = q.Get("toSelector")
		if d.present("toX") {
			d.Float("toX", &req.ToX)
			req.HasToXY = true
		}
		if d.present("toY") {
			d.Float("toY", &req.ToY)
			req.HasToXY = true
		}
		req.Button = q.Get("button")
		d.Bool("dismissBanners", &req.DismissBanners)
		d.Bool("dismissKnownInterstitials", &req.DismissKnownInterstitials)
		d.Bool("submit", &req.Submit)
		if d.present("autoSwitch") {
			var autoSwitch bool
			d.Bool("autoSwitch", &autoSwitch)
			req.AutoSwitch = &autoSwitch
		}
		if d.present("scrollX") {
			d.Int("scrollX", &req.ScrollX)
			req.HasScroll = true
		}
		if d.present("scrollY") {
			d.Int("scrollY", &req.ScrollY)
			req.HasScroll = true
		}
		var hasScrollParam bool
		d.Bool("hasScroll", &hasScrollParam)
		req.HasScroll = req.HasScroll || hasScrollParam
		if d.present("deltaX") {
			d.Int("deltaX", &req.DeltaX)
			req.HasDelta = true
		}
		if d.present("deltaY") {
			d.Int("deltaY", &req.DeltaY)
			req.HasDelta = true
		}
		var hasDeltaParam bool
		d.Bool("hasDelta", &hasDeltaParam)
		req.HasDelta = req.HasDelta || hasDeltaParam
		req.Browser = q.Get("browser")
		req.Vocab = q.Get("vocab")
		if err := d.Err(); err != nil {
			httpx.Error(w, 400, err)
			return bridge.ActionRequest{}, false
		}
		return req, true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return bridge.ActionRequest{}, false
	}
	req.Kind = bridge.CanonicalActionKind(req.Kind)
	req.DialogAction = strings.ToLower(strings.TrimSpace(req.DialogAction))
	return req, true
}

const vocabHeader = "X-PinchTab-Vocab"

const vocabSupersededCode = "vocab_superseded"

func actionTargetsRef(req bridge.ActionRequest) bool {
	if req.NodeID != 0 {
		return false
	}
	if strings.TrimSpace(req.Ref) != "" {
		return true
	}
	return selector.Parse(req.Selector).Kind == selector.KindRef
}

func writeVocabSuperseded(w http.ResponseWriter, tabID string) {
	httpx.ErrorCode(w, http.StatusConflict, vocabSupersededCode,
		"ref vocabulary superseded: a newer snapshot renumbered this tab's refs, so a ref from the earlier snapshot no longer denotes the node it named — re-snapshot and use the refs from the latest response",
		true, map[string]any{"tabId": tabID})
}

func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeActionRequest(w, r)
	if !ok {
		return
	}

	routing, ok := h.resolveBrowserForRequest(w, r, req.TabID, strings.TrimSpace(req.Browser), browsers.RequestIntent{
		Shape:         browsers.ShapeInteraction,
		StateChanging: true,
	})
	if !ok {
		return
	}
	resolvedBrowser := routing.Browser
	effectiveCfg := routing.EffectiveCfg

	req.Browser = resolvedBrowser

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
	// Here rather than only inside the bridge action: the ghost-chrome proxy answers
	// fill from its static browser before the Chrome action runs, so a check living in
	// actionFill alone would be bypassed for one provider.
	if err := bridge.ValidateFillAction(req.Kind, req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_fill_text", err.Error(), false, nil)
		return
	}
	if err := bridge.ValidateButtonAction(req.Kind, req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_mouse_button", err.Error(), false, nil)
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
		if _, ok := h.applyTabGuards(w, r, ctx, resolvedTabID, guardDialogBlocked|guardDomainPolicy|guardHandoffPause); !ok {
			return
		}
		defer h.armAutoCloseIfEnabled(resolvedTabID)
	}
	h.recordResolvedTab(r, resolvedTabID)
	w.Header().Set(activity.HeaderPTTabID, resolvedTabID)

	if req.Vocab == "" {
		req.Vocab = r.Header.Get(vocabHeader)
	}
	if req.Vocab != "" && actionTargetsRef(req) {
		if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil && cache.DomEpoch != "" && cache.DomEpoch != req.Vocab {
			writeVocabSuperseded(w, resolvedTabID)
			return
		}
	}

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
		h.errorWithCrashContext(w, selectorResolution.httpStatus(), err)
		return
	}
	destinationResolution, err := h.resolveActionRequestDestination(tCtx, resolvedTabID, &req)
	if err != nil {
		h.errorWithCrashContext(w, destinationResolution.httpStatus(), err)
		return
	}
	// Both ends of a drag come from one snapshot, so a stale destination gets the
	// same refresh-and-recover the source does instead of an unconditional 404;
	// only a destination that still cannot be found refuses, naming the
	// destination. A destination that resolved is intent-cached like the source,
	// so a later recovery refresh can descriptor-match it rather than trusting a
	// positional ref against a new snapshot.
	if destinationResolution.refMissing {
		h.refreshRefCache(tCtx, resolvedTabID)
		if err := h.refreshActionSecondaryTargets(tCtx, resolvedTabID, &req); err != nil {
			writeTargetNotFound(w, err, nil)
			return
		}
	} else if req.ToSelector != "" && req.ToNodeID != 0 && h.Recovery != nil {
		if toSel := selector.Parse(req.ToSelector); toSel.Kind == selector.KindRef {
			h.cacheActionIntent(resolvedTabID, bridge.ActionRequest{Ref: toSel.Value})
		}
	}
	refMissing := selectorResolution.refMissing
	submitClick := bridge.IsSubmitClick(req.Kind, req)
	if submitClick && refMissing {
		httpx.ErrorCode(w, http.StatusNotFound, "submit_target_not_found", refNotFound(req.Ref).Error(), false, staleSubmitTargetDetails())
		return
	}
	if submitClick && req.NodeID <= 0 {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_submit_target", "click submit requires a selector, ref, or nodeId", false, map[string]any{
			"dispatched": false,
		})
		return
	}

	if req.Ref != "" && h.Recovery != nil && !refMissing {
		h.cacheActionIntent(resolvedTabID, req)
	}

	if refMissing && h.Recovery == nil {
		writeTargetNotFound(w, refNotFound(req.Ref), nil)
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
	submitTimeoutWithDialog := submitClick && isTimeoutWithPendingDialog(actionErr, resolvedTabID, h.Bridge)
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
			message := fmt.Sprintf("%s - valid values: %s", actionErr.Error(), strings.Join(kinds, ", "))
			httpx.JSONError(w, 400, "unknown_action_kind", message, map[string]string{"error": message})
			return
		}
		if errors.Is(actionErr, bridge.ErrInvalidActionRequest) {
			httpx.ErrorCode(w, http.StatusBadRequest, "invalid_action_request", fmt.Sprintf("action %s: %v", req.Kind, actionErr), false, nil)
			return
		}
		if errors.Is(actionErr, ErrStaleSubmitTarget) {
			httpx.ErrorCode(w, http.StatusNotFound, "submit_target_not_found",
				refNotFound(req.Ref).Error(), false, staleSubmitTargetDetails())
			return
		}
		if browserops.IsIDPIBlocked(actionErr) {
			httpx.ErrorCode(w, http.StatusForbidden, "idpi_blocked", actionErr.Error(), false, nil)
			return
		}
		if message, dialog, ok := h.mapDialogBlockingError(actionErr, req.Kind, resolvedTabID); ok {
			writeDialogBlocked(w, resolvedTabID, dialog, message)
			return
		}
		if errors.Is(actionErr, ErrTargetNotFound) {
			writeTargetNotFound(w, actionErr, recoveryResult)
			return
		}
		dispatchMayHaveLanded := submitClick
		var details map[string]any
		if dispatchMayHaveLanded {
			details = map[string]any{
				"dispatch":   "unconfirmed",
				"doNotRetry": true,
			}
		}
		h.errorCodeWithCrashContext(w, 500, "action_failed", fmt.Sprintf("action %s: %v", req.Kind, actionErr), actionFailureIsRetryable(actionErr, dispatchMayHaveLanded), details)
		return
	}

	if actionBackend == "" {
		actionBackend = "chrome"
	}
	if actionBackend != "static" {
		h.maybeAutoSolve(tCtx, resolvedTabID, autoSolverTriggerAction)
		if req.WaitNav && req.DismissBanners {
			h.dismissBanners(tCtx, resolvedTabID, true)
		}
	}
	if switched := switchedTabFromActionResult(result); switched != "" {
		h.setCurrentTabForRequest(r, switched)
		markCreatedTab(w, switched)
		h.recordResolvedTab(r, switched)
	}
	actionRoute := routeMetadataFor(routing)
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
		if _, ok := h.applyTabGuards(w, r, ctx, resolvedTabID, guardDomainPolicy); !ok {
			return
		}
		if err := h.enforceTabNotPausedForHandoff(resolvedTabID); err != nil {
			results = append(results, h.handoffPausedActionResult(i, resolvedTabID, err))
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

	batchRoute := routeMetadataFor(routing)
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
			Error: refNotFound(step.Ref).Error(),
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
	macroIntentBrowser := macroResolvedBrowser
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
		if _, ok := h.applyTabGuards(w, r, ctx, resolvedTabID, guardDomainPolicy); !ok {
			return
		}
		if err := h.enforceTabNotPausedForHandoff(resolvedTabID); err != nil {
			results = append(results, h.handoffPausedActionResult(i, resolvedTabID, err))
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

	macroRoute := routeMetadataFor(browserRouting{
		Browser:        macroResolvedBrowser,
		IntentBrowser:  macroIntentBrowser,
		RequestBrowser: macroRequestBrowser,
		EffectiveCfg:   macroEffectiveCfg,
		Decision:       macroHandleDecision,
	})
	h.writeMultiStepActionResult(w, r, ctx, resolvedTabID, results, len(req.Steps), macroRoute, map[string]any{"kind": "macro"})
}
