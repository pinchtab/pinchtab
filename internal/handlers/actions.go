package handlers

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/semantic/recovery"
)

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
func writeTargetNotFound(w http.ResponseWriter, tabID string, err error, rr *recovery.RecoveryResult) {
	details := map[string]any{"dispatched": false, "tabId": tabID}
	if rr != nil {
		details["recovery"] = rr
	}
	httpx.ErrorCode(w, http.StatusNotFound, CodeRefNotFound, err.Error(), false, details)
}

// writeNoOptionMatch classifies a select whose value matched no option as a
// client error: the element was found and resolved, so retrying the identical
// request can never succeed. details.available carries the structured option
// list, and details.hint renders it through the CLI's generic guidance path so
// the caller sees the choices without re-inspecting the page.
func writeNoOptionMatch(w http.ResponseWriter, kind string, err error, noOption *bridgecdpops.NoOptionMatchError) {
	httpx.ErrorCode(w, http.StatusUnprocessableEntity, "option_not_found",
		fmt.Sprintf("action %s: %v", kind, err), false, map[string]any{
			"available": noOption.Available,
			"hint":      availableOptionsHint(noOption.Available),
		})
}

func (h *Handlers) invalidActionDetails(actionErr error, tabID string) map[string]any {
	var closedOption *bridge.ClosedSelectOptionError
	if !errors.As(actionErr, &closedOption) {
		return nil
	}
	return map[string]any{
		"dispatched": false,
		"hint":       fmt.Sprintf("choose the option on its <select> instead: select %s %s", h.refForNode(tabID, closedOption.SelectBackendNodeID), shellQuote(closedOption.OptionText)),
	}
}

func (h *Handlers) refForNode(tabID string, backendNodeID int64) string {
	if cache := h.Bridge.GetRefCache(tabID); cache != nil {
		for ref, target := range cache.Targets {
			if target.BackendNodeID == backendNodeID {
				return ref
			}
		}
	}
	return "<select ref>"
}

func availableOptionsHint(options []bridgecdpops.SelectOption) string {
	if len(options) == 0 {
		return "the <select> has no options"
	}
	rendered := make([]string, len(options))
	for i, o := range options {
		if o.Text == "" || o.Text == o.Value {
			rendered[i] = shellQuote(o.Value)
			continue
		}
		rendered[i] = fmt.Sprintf("%s (%s)", shellQuote(o.Value), shellQuote(o.Text))
	}
	return "available options: " + strings.Join(rendered, ", ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
	if dialog := dialogPendingAfterTimeout(err, tabID, h.Bridge); dialog != nil {
		return fmt.Sprintf("action %s timed out; a JavaScript dialog is blocking (%s: %q)", kind, dialog.Type, dialog.Message), dialog, true
	}
	return "", nil, false
}

// dialogOpenedByInput names the dialog an input action opened. Every action
// route refuses a tab whose dialog is already open before dispatching, so a
// dialog pending when dispatched input times out was opened by that input.
func (h *Handlers) dialogOpenedByInput(req *bridge.ActionRequest, tabID string, result map[string]any, err error) *bridge.DialogState {
	if dialog := bridge.OpenedDialog(result); dialog != nil {
		return dialog
	}
	if !bridge.DispatchesInput(*req) {
		return nil
	}
	return dialogPendingAfterTimeout(err, tabID, h.Bridge)
}

func (h *Handlers) failedStep(index int, kind, tabID string, err error, fallback string) actionResult {
	if message, _, ok := h.mapDialogBlockingError(err, kind, tabID); ok {
		fallback = dialogBlockedStepError(message)
	}
	return actionResult{Index: index, Success: false, Error: fallback}
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
	defer h.armFormValidationWatch(ctx, tCtx, *step, false)()
	res, _, _, err := h.executeActionResilient(tCtx, step, cfg, tabID, refMissing)
	if dialog := h.dialogOpenedByInput(step, tabID, res, err); dialog != nil {
		return actionResult{Index: index, Success: true, Result: withOpenedDialog(res, tabID, dialog)}, ctx, tabID
	}
	if err == nil {
		res = h.settleWaitNavClick(tCtx, *step, res)
	}
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
				h.markCreatedTab(w, r, nextTabID)
				h.recordResolvedTab(r, nextTabID)
			}
		}
	}
	if err != nil {
		return h.failedStep(index, step.Kind, nextTabID, err, errFallback(err)), nextCtx, nextTabID
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
	for _, field := range reflect.VisibleFields(reflect.TypeFor[bridge.ActionRequest]()) {
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

// batchActionQueryKeys is the accepted query-parameter set for the batch POST
// /actions endpoint: only the keys this route actually reads from the query. The
// batch takes its steps from the JSON body and its target tab from the
// body/path/current-tab rule — never from the query — so the one meaningful query
// key is owner, read by resolveOwner (the r.URL.Query().Get("owner") near the top
// of this file). Everything else would be silently dropped: a per-step action
// field, a mistargeted ?tab= or ?tabId= (the batch ignores a query tabId and would
// run on the current tab), any typo. Each is refused with the same message
// /action uses. Pinned as a subset of actionQueryKeys by
// TestBatchActionQueryKeysAreActionParameters.
var batchActionQueryKeys = map[string]struct{}{
	"owner": {},
}

// postActionQueryKeys is the accepted query-parameter set for POST /action. The body
// is authoritative, so the handler reads nothing else from the query except owner
// (resolveOwner) and browser — MCP's routedPathWithBody appends ?browser= to every
// POST /action and the multi-instance router reads it to pick the instance, so
// refusing it would 400 a browser-targeted MCP action. Every other query key would be
// silently dropped — a mistargeted ?tab=/?tabId= running on the current tab, a typo —
// so it is refused with the same message the batch route uses. Pinned as a subset of
// actionQueryKeys by TestPostActionQueryKeysAreActionParameters.
var postActionQueryKeys = map[string]struct{}{
	"owner":   {},
	"browser": {},
}

// unknownQueryFields names every supplied parameter not in the known set, sorted
// so the refusal reads the same on every run. Presence follows the decoder's own
// rule — a non-empty value — so ?_= is absent rather than an unknown request.
func unknownQueryFields(q url.Values, known map[string]struct{}) []string {
	var unknown []string
	for key := range q {
		if _, ok := known[key]; ok {
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
		if unknown := unknownQueryFields(q, actionQueryKeys); len(unknown) > 0 {
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
		req.VocabTab = q.Get("vocabTab")
		if err := d.Err(); err != nil {
			httpx.Error(w, 400, err)
			return bridge.ActionRequest{}, false
		}
		return req, true
	}
	// POST takes its parameters from the JSON body, so a query parameter here is
	// silently dropped — a mistargeted ?tab=/?tabId= would run on the current tab, the
	// same wrong-tab-write hazard the batch route refuses. Refuse any stray key before
	// the action runs, matching the batch route's message and its owner/browser
	// exceptions (browser is what MCP appends and the multi-instance router reads).
	if unknown := unknownQueryFields(r.URL.Query(), postActionQueryKeys); len(unknown) > 0 {
		httpx.Error(w, 400, unknownQueryFieldsError(unknown))
		return bridge.ActionRequest{}, false
	}
	if !decodeRequiredJSON(w, r, &req) {
		return bridge.ActionRequest{}, false
	}
	req.Kind = bridge.CanonicalActionKind(req.Kind)
	req.DialogAction = strings.ToLower(strings.TrimSpace(req.DialogAction))
	return req, true
}

const vocabHeader = types.HeaderVocab

const vocabSupersededCode = "vocab_superseded"

func vocabTabMatches(vocabTab, resolvedTabID string) bool {
	return vocabTab == "" || vocabTab == resolvedTabID
}

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

	if !validateActionRequestFields(w, req) {
		return
	}
	h.recordActionRequest(r, req)
	if !h.actionKindKnown(req.Kind) {
		h.writeUnknownActionKind(w, req.Kind)
		return
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
		if _, ok := h.applyTabGuardsAs(w, r, ctx, resolvedTabID, req.Owner, guardTabLease|guardDialogBlocked|guardDomainPolicy|guardHandoffPause); !ok {
			return
		}
		defer h.armIdleLifecycle(resolvedTabID)
	}
	h.recordResolvedTab(r, resolvedTabID)
	w.Header().Set(activity.HeaderPTTabID, resolvedTabID)

	if req.Vocab == "" {
		req.Vocab = r.Header.Get(vocabHeader)
	}
	if req.Vocab != "" && actionTargetsRef(req) && vocabTabMatches(req.VocabTab, resolvedTabID) {
		if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil && cache.DomEpoch != "" && cache.DomEpoch != req.Vocab {
			writeVocabSuperseded(w, resolvedTabID)
			return
		}
	}

	vocabBefore := h.tabVocab(resolvedTabID)

	actionTimeout, err := actionTimeoutFor(r, effectiveCfg.ActionTimeout)
	if err != nil {
		writeInvalidTimeout(w, err)
		return
	}
	tCtx, tCancel := httpx.WithClientTimeout(r.Context(), ctx, actionTimeout)
	defer tCancel()

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
	if !h.resolveActionDestination(w, tCtx, resolvedTabID, &req) {
		return
	}
	refMissing := selectorResolution.refMissing
	submitClick := bridge.IsSubmitClick(req.Kind, req)
	if submitClick && !validateSubmitTarget(w, req, refMissing) {
		return
	}

	if req.Ref != "" && h.Recovery != nil && !refMissing {
		h.cacheActionIntent(resolvedTabID, req)
	}

	if refMissing && h.Recovery == nil {
		writeTargetNotFound(w, resolvedTabID, h.refNotFoundOn(resolvedTabID, req.Ref), nil)
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
	defer h.armFormValidationWatch(ctx, tCtx, req, submitClick)()

	result, actionBackend, recoveryResult, actionErr := h.executeActionResilient(tCtx, &req, effectiveCfg, resolvedTabID, refMissing)
	h.publishVocabIfReepoched(w, resolvedTabID, vocabBefore)
	if dialog := h.dialogOpenedByInput(&req, resolvedTabID, result, actionErr); dialog != nil {
		h.writeActionResult(w, r, routing, withOpenedDialog(result, resolvedTabID, dialog), recoveryResult)
		return
	}
	if submitClick && (actionErr == nil || errors.Is(actionErr, context.DeadlineExceeded)) {
		if result, ok = h.runSubmitClick(w, r, ctx, resolvedTabID, req.NodeID, submitBefore, result, actionErr); !ok {
			return
		}
		actionErr = nil
	}
	if actionErr != nil {
		h.writeActionError(w, actionErr, req, resolvedTabID, recoveryResult, submitClick)
		return
	}
	result = h.settleWaitNavClick(tCtx, req, result)
	h.writeActionSuccess(w, r, tCtx, req, routing, resolvedTabID, result, actionBackend, recoveryResult)
}

func isWaitNavClick(req bridge.ActionRequest) bool {
	return req.WaitNav && req.Kind == bridge.ActionClick
}

func watchesFormValidation(req bridge.ActionRequest, submitClick bool) bool {
	return submitClick || (isWaitNavClick(req) && req.NodeID > 0)
}

func (h *Handlers) armFormValidationWatch(tabCtx, tCtx context.Context, req bridge.ActionRequest, submitClick bool) (disarm func()) {
	if !watchesFormValidation(req, submitClick) {
		return func() {}
	}
	h.armSubmitInvalidWatch(tCtx, req.NodeID)
	return func() { h.disarmSubmitInvalidWatch(tabCtx, req.NodeID) }
}

func (h *Handlers) settleWaitNavClick(ctx context.Context, req bridge.ActionRequest, result map[string]any) map[string]any {
	if !isWaitNavClick(req) {
		return result
	}
	return h.settleWaitNavOutcome(ctx, req.NodeID, result)
}

func validateActionRequestFields(w http.ResponseWriter, req bridge.ActionRequest) bool {
	if req.Kind == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'kind'"))
		return false
	}
	if req.DialogAction != "" && req.DialogAction != "accept" && req.DialogAction != "dismiss" {
		httpx.Error(w, 400, fmt.Errorf("dialogAction must be 'accept' or 'dismiss'"))
		return false
	}
	if err := bridge.ValidateSubmitAction(req.Kind, req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_submit_action", err.Error(), false, nil)
		return false
	}
	// Here rather than only inside the bridge action: the ghost-chrome proxy answers
	// fill from its static browser before the Chrome action runs, so a check living in
	// actionFill alone would be bypassed for one provider.
	if err := bridge.ValidateFillAction(req.Kind, req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_fill_text", err.Error(), false, nil)
		return false
	}
	if err := bridge.ValidateButtonAction(req.Kind, req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_mouse_button", err.Error(), false, nil)
		return false
	}
	return true
}

func actionTimeoutFor(r *http.Request, configured time.Duration) (time.Duration, error) {
	if r.Method != http.MethodGet {
		return configured, nil
	}
	v := r.URL.Query().Get("timeout")
	if v == "" {
		return configured, nil
	}
	return parseTimeoutSeconds("timeout", v)
}

// Both ends of a drag come from one snapshot, so a stale destination gets the
// same refresh-and-recover the source does instead of an unconditional 404;
// only a destination that still cannot be found refuses, naming the
// destination. A destination that resolved is intent-cached like the source,
// so a later recovery refresh can descriptor-match it rather than trusting a
// positional ref against a new snapshot.
func (h *Handlers) resolveActionDestination(w http.ResponseWriter, ctx context.Context, tabID string, req *bridge.ActionRequest) bool {
	destinationResolution, err := h.resolveActionRequestDestination(ctx, tabID, req)
	if err != nil {
		h.errorWithCrashContext(w, destinationResolution.httpStatus(), err)
		return false
	}
	if destinationResolution.refMissing {
		h.refreshRefCache(ctx, tabID)
		if err := h.refreshActionSecondaryTargets(ctx, tabID, req); err != nil {
			writeTargetNotFound(w, tabID, err, nil)
			return false
		}
	} else if req.ToSelector != "" && req.ToNodeID != 0 && h.Recovery != nil {
		if toSel := selector.Parse(req.ToSelector); toSel.Kind == selector.KindRef {
			h.cacheActionIntent(tabID, bridge.ActionRequest{Ref: toSel.Value})
		}
	}
	return true
}

func validateSubmitTarget(w http.ResponseWriter, req bridge.ActionRequest, refMissing bool) bool {
	if refMissing {
		httpx.ErrorCode(w, http.StatusNotFound, "submit_target_not_found", refNotFound(req.Ref).Error(), false, staleSubmitTargetDetails())
		return false
	}
	if req.NodeID <= 0 {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_submit_target", "click submit requires a selector, ref, or nodeId", false, map[string]any{
			"dispatched": false,
		})
		return false
	}
	return true
}

func (h *Handlers) runSubmitClick(w http.ResponseWriter, r *http.Request, tabCtx context.Context, tabID string, nodeID int64, before submitStateSnapshot, result map[string]any, actionErr error) (map[string]any, bool) {
	actionTimedOut := errors.Is(actionErr, context.DeadlineExceeded)
	dispatch := "acknowledged"
	if actionTimedOut {
		dispatch = "unconfirmed"
	}

	// The action's context may be expired here. Keep the live tab context as the parent
	// and give post-state observation its own bounded, client-cancelable child.
	postCtx, postCancel := httpx.WithClientTimeout(r.Context(), tabCtx, postSubmitPollTimeout)
	postState, postErr := h.pollSubmitPostState(postCtx, tabID, nodeID, before, dispatch, actionTimedOut)
	postCancel()
	if postErr != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "submit_post_state_failed", postErr.Error(), false, map[string]any{
			"dispatch":       dispatch,
			"actionTimedOut": actionTimedOut,
			"doNotRetry":     true,
		})
		return nil, false
	}
	if result == nil {
		result = make(map[string]any)
	}
	result["postState"] = postState
	return result, true
}

func (h *Handlers) writeActionError(w http.ResponseWriter, actionErr error, req bridge.ActionRequest, resolvedTabID string, recoveryResult *recovery.RecoveryResult, submitClick bool) {
	if errors.Is(actionErr, bridge.ErrUnknownAction) {
		h.writeUnknownActionKind(w, req.Kind)
		return
	}
	if errors.Is(actionErr, bridge.ErrInvalidActionRequest) {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_action_request", fmt.Sprintf("action %s: %v", req.Kind, actionErr), false, h.invalidActionDetails(actionErr, resolvedTabID))
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
		writeTargetNotFound(w, resolvedTabID, actionErr, recoveryResult)
		return
	}
	var noOption *bridgecdpops.NoOptionMatchError
	if errors.As(actionErr, &noOption) {
		writeNoOptionMatch(w, req.Kind, actionErr, noOption)
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
}

func (h *Handlers) writeActionSuccess(w http.ResponseWriter, r *http.Request, tCtx context.Context, req bridge.ActionRequest, routing browserRouting, resolvedTabID string, result map[string]any, actionBackend string, recoveryResult *recovery.RecoveryResult) {
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
		h.markCreatedTab(w, r, switched)
		h.recordResolvedTab(r, switched)
	}
	h.writeActionResult(w, r, routing, result, recoveryResult)
}

func (h *Handlers) writeActionResult(w http.ResponseWriter, r *http.Request, routing browserRouting, result map[string]any, recoveryResult *recovery.RecoveryResult) {
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

func (h *Handlers) actionKindKnown(kind string) bool {
	available := h.Bridge.AvailableActions()
	if len(available) == 0 {
		return true
	}
	return slices.Contains(available, kind)
}

func (h *Handlers) writeUnknownActionKind(w http.ResponseWriter, kind string) {
	message := fmt.Sprintf("%s: %s", bridge.ErrUnknownAction, kind)
	var details map[string]any
	if available := h.Bridge.AvailableActions(); len(available) > 0 {
		kinds := append([]string(nil), available...)
		sort.Strings(kinds)
		message += " - valid values: " + strings.Join(kinds, ", ")
		details = map[string]any{"validKinds": kinds}
	}
	httpx.ErrorCode(w, http.StatusBadRequest, "unknown_action_kind", message, false, details)
}

func (h *Handlers) HandleActions(w http.ResponseWriter, r *http.Request) {
	// Refuse a stray or mistargeted query parameter before any step runs, matching
	// the singular /action: the batch takes its steps and tab from the JSON body, so
	// a ?tab= (or any unknown key) would be silently dropped and the batch would run
	// on the current tab — a silent wrong-tab write.
	if unknown := unknownQueryFields(r.URL.Query(), batchActionQueryKeys); len(unknown) > 0 {
		httpx.Error(w, 400, unknownQueryFieldsError(unknown))
		return
	}

	var req actionsRequest
	if !decodeRequiredJSON(w, r, &req) {
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

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.applyTabGuardsAs(w, r, ctx, resolvedTabID, req.Owner, guardTabLease|guardDialogBlocked); !ok {
		return
	}

	run, ctx, resolvedTabID, ok := h.runMultiStep(w, r, ctx, resolvedTabID, multiStepPlan{
		steps:       req.Actions,
		owner:       req.Owner,
		stopOnError: req.StopOnError,
		stepTimeout: effectiveCfg.ActionTimeout,
		cfg:         effectiveCfg,
		errFmt: func(kind string, err error) string {
			return fmt.Sprintf("action %s: %v", kind, err)
		},
	})
	if !ok {
		return
	}

	batchRoute := routeMetadataFor(routing)
	h.publishMultiStepVocab(w, run, resolvedTabID)
	h.writeMultiStepActionResult(w, r, ctx, resolvedTabID, run.results, len(req.Actions), batchRoute, nil)
}

type multiStepPlan struct {
	steps       []bridge.ActionRequest
	owner       string
	stopOnError bool
	stepTimeout time.Duration
	cfg         *config.RuntimeConfig
	errFmt      func(kind string, err error) string
}

// runMultiStep is the one step loop behind /actions and /macro. A step naming
// another tab switches the run to it; ok=false means a guard already answered.
func (h *Handlers) runMultiStep(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string, plan multiStepPlan) (*multiStepRun, context.Context, string, bool) {
	run := newMultiStepRun(len(plan.steps), plan.stopOnError)
	for i := 0; i < len(plan.steps) && !run.stopped; i++ {
		step := plan.steps[i]
		if step.TabID == "" {
			step.TabID = tabID
		} else if step.TabID != tabID {
			stepCtx, stepTabID, err := h.stepTabContext(r, step.TabID, plan.owner)
			if err != nil {
				run.record(actionResult{Index: i, Success: false, Error: err.Error()})
				continue
			}
			ctx, tabID = stepCtx, stepTabID
		}
		run.enterTab(tabID, h.tabVocab(tabID))
		if _, ok := h.applyTabGuards(w, r, ctx, tabID, guardDomainPolicy); !ok {
			return nil, ctx, tabID, false
		}
		if dialog := pendingTabDialog(h.Bridge, tabID); dialog != nil {
			run.stop(dialogBlockedActionResult(i, tabID, dialog))
			continue
		}
		if err := h.enforceTabNotPausedForHandoff(tabID); err != nil {
			run.record(h.handoffPausedActionResult(i, tabID, err))
			continue
		}

		tCtx, cancel := context.WithTimeout(ctx, plan.stepTimeout)
		var result actionResult
		result, ctx, tabID = h.runMultiStepAction(ctx, tCtx, r, w, &step, plan, tabID, i)
		cancel()
		run.record(result)
	}
	return run, ctx, tabID, true
}

func (h *Handlers) runMultiStepAction(
	ctx, tCtx context.Context,
	r *http.Request, w http.ResponseWriter,
	step *bridge.ActionRequest, plan multiStepPlan,
	tabID string, index int,
) (actionResult, context.Context, string) {
	selectorResolution, resolveErr := h.resolveActionRequestSelector(tCtx, tabID, step)
	if resolveErr != nil {
		return h.failedStep(index, step.Kind, tabID, resolveErr, resolveErr.Error()), ctx, tabID
	}
	if step.Kind == "" {
		return actionResult{Index: index, Success: false, Error: "missing required field 'kind'"}, ctx, tabID
	}
	refMissing := selectorResolution.refMissing
	if step.Ref != "" && h.Recovery != nil && !refMissing {
		h.cacheActionIntent(tabID, *step)
	}
	if refMissing && h.Recovery == nil {
		return actionResult{Index: index, Success: false, Error: h.refNotFoundOn(tabID, step.Ref).Error()}, ctx, tabID
	}
	return h.runResolvedActionStep(ctx, tCtx, r, w, step, plan.cfg, tabID, index, refMissing, func(err error) string {
		return plan.errFmt(step.Kind, err)
	})
}

type multiStepRun struct {
	results     []actionResult
	stopOnError bool
	stopped     bool
	vocabBefore map[string]string
}

func newMultiStepRun(steps int, stopOnError bool) *multiStepRun {
	return &multiStepRun{results: make([]actionResult, 0, steps), stopOnError: stopOnError, vocabBefore: map[string]string{}}
}

func (m *multiStepRun) enterTab(tabID, vocab string) {
	if _, seen := m.vocabBefore[tabID]; !seen {
		m.vocabBefore[tabID] = vocab
	}
}

func (h *Handlers) publishMultiStepVocab(w http.ResponseWriter, run *multiStepRun, tabID string) {
	if before, seen := run.vocabBefore[tabID]; seen {
		h.publishVocabIfReepoched(w, tabID, before)
	}
}

func (m *multiStepRun) record(result actionResult) {
	m.results = append(m.results, result)
	if !result.Success && m.stopOnError {
		m.stopped = true
	}
}

func (m *multiStepRun) stop(result actionResult) {
	m.results = append(m.results, result)
	m.stopped = true
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
	failed := total - successful
	if successful > 0 {
		h.maybeAutoSolve(ctx, resolvedTabID, autoSolverTriggerAction)
	}
	h.recordActivity(r, activity.Update{
		Route: route,
		Steps: &activity.StepCounts{Total: total, Successful: successful, Failed: failed},
	})
	// The envelope answers 200 whatever happened inside it — that contract is
	// deliberate and the per-item results are correct — so the failure signal
	// travels on the channel built for it instead. Metrics, slog and the activity
	// record all read the recorded reason, and without this a run in which every
	// step failed is indistinguishable from one in which every step succeeded.
	if reason := multiStepFailureReason(results, total, successful); reason.message != "" {
		httpx.RecordFailureReason(w, reason.code, reason.message)
	}
	resp := map[string]any{
		"results":    results,
		"total":      total,
		"successful": successful,
		"failed":     failed,
		"route":      route,
	}
	maps.Copy(resp, extra)
	httpx.JSON(w, 200, resp)
}

type multiStepFailure struct {
	code    string
	message string
}

// multiStepFailureReason summarises a run for the recording channels: how many of
// how many steps failed, and the first failure's own code and message, which is
// the one a reader needs to act — the later ones are usually its consequence.
// A run with no failures records nothing, so a healthy batch stays at INFO and
// moves no counter.
func multiStepFailureReason(results []actionResult, total, successful int) multiStepFailure {
	failed := total - successful
	if failed <= 0 {
		return multiStepFailure{}
	}
	first := multiStepFailure{code: "multi_step_failed"}
	for _, result := range results {
		if result.Success {
			continue
		}
		if result.Code != "" {
			first.code = result.Code
		}
		first.message = result.Error
		break
	}
	summary := fmt.Sprintf("%d of %d steps failed", failed, total)
	if first.message != "" {
		summary += ": " + first.message
	}
	return multiStepFailure{code: first.code, message: summary}
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
	if !decodeRequiredJSON(w, r, &req) {
		return
	}
	if len(req.Steps) == 0 {
		httpx.ErrorCode(w, 400, "bad_request", "steps array is empty", false, nil)
		return
	}
	if !rejectMultiStepSubmitClicks(w, req.Steps, "macro", "steps") {
		return
	}
	macroRequestBrowser, ok := rejectMixedBrowsers(w, req.Steps, "macro", "steps")
	if !ok {
		return
	}
	routing, ok := h.resolveBrowserForRequest(w, r, req.TabID, macroRequestBrowser, browsers.RequestIntent{
		Shape:         browsers.ShapeInteraction,
		StateChanging: true,
	})
	if !ok {
		return
	}

	stepTimeout := routing.EffectiveCfg.ActionTimeout
	if req.StepTimeout != 0 {
		parsed, err := timeoutFromSeconds("stepTimeout", req.StepTimeout)
		if err != nil {
			writeInvalidTimeout(w, err)
			return
		}
		stepTimeout = parsed
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.applyTabGuardsAs(w, r, ctx, resolvedTabID, req.Owner, guardTabLease|guardDialogBlocked); !ok {
		return
	}

	run, ctx, resolvedTabID, ok := h.runMultiStep(w, r, ctx, resolvedTabID, multiStepPlan{
		steps:       req.Steps,
		owner:       req.Owner,
		stopOnError: req.StopOnError,
		stepTimeout: stepTimeout,
		cfg:         routing.EffectiveCfg,
		errFmt: func(_ string, err error) string {
			return err.Error()
		},
	})
	if !ok {
		return
	}

	h.publishMultiStepVocab(w, run, resolvedTabID)
	h.writeMultiStepActionResult(w, r, ctx, resolvedTabID, run.results, len(req.Steps), routeMetadataFor(routing), map[string]any{"kind": "macro"})
}
