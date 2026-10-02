package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

const (
	postSubmitPollTimeout  = 3 * time.Second
	postSubmitPollInterval = 250 * time.Millisecond

	submitLiveRegionTextLimit = 500
)

const submitInvalidWatchKeyJS = `Symbol.for("pinchtab.submitInvalidWatch")`

// A click resolved by text lands on a <span> inside the button, which has no form of its own.
const submitControlOfNodeJS = `(this.closest ? this.closest("button, input") : null)`

// The watch listens on the form's root in the capture phase: invalid events do not bubble,
// and a control tied to the form by a form= attribute sits outside the form's subtree.
const armSubmitInvalidWatchJS = `function(requireSubmitter) {
	const key = ` + submitInvalidWatchKeyJS + `;
	if (this[key]) this[key].disarm();
	const control = ` + submitControlOfNodeJS + `;
	const submitter = control && (control.type === "submit" || control.type === "image");
	const form = control && (submitter || !requireSubmitter) && control.form;
	if (!form) return false;
	const root = form.getRootNode();
	const watch = { fired: false };
	const onInvalid = (event) => { if (event.target.form === form) watch.fired = true; };
	watch.disarm = () => root.removeEventListener("invalid", onInvalid, true);
	root.addEventListener("invalid", onInvalid, true);
	this[key] = watch;
	return true;
}`

const disarmSubmitInvalidWatchJS = `function() {
	const key = ` + submitInvalidWatchKeyJS + `;
	if (!this[key]) return;
	this[key].disarm();
	delete this[key];
}`

// Reads validity instead of calling checkValidity, which would fire invalid events of its own.
const submitInvalidFieldsJS = `function() {
	const watch = this[` + submitInvalidWatchKeyJS + `];
	const control = ` + submitControlOfNodeJS + `;
	const form = control && control.form;
	if (!watch || !watch.fired || !form) return [];
	const label = (el) => ((el.labels && el.labels[0] && el.labels[0].innerText) || el.getAttribute("aria-label") || el.placeholder || "").replace(/\s+/g, " ").trim().slice(0, 100);
	return Array.from(form.elements)
		.filter((el) => el.willValidate && !el.validity.valid)
		.slice(0, 10)
		.map((el) => ({ name: el.name || el.id || "", label: label(el), message: (el.validationMessage || "").slice(0, 200) }));
}`

const submitLiveRegionsJS = `(() => {
	const live = '[role="status"], [role="alert"], [aria-live="polite"], [aria-live="assertive"], output';
	const rendered = (el) => el.checkVisibility
		? el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })
		: el.getClientRects().length > 0;
	const texts = [];
	const visit = (root) => {
		for (const el of root.querySelectorAll(live)) {
			if (el.getAttribute("aria-live") === "off" || !rendered(el)) continue;
			if (el.parentElement && el.parentElement.closest(live)) continue;
			const text = (el.innerText || el.textContent || "").replace(/\s+/g, " ").trim();
			if (text) texts.push(text.slice(0, 200));
		}
		for (const el of root.querySelectorAll("*")) if (el.shadowRoot) visit(el.shadowRoot);
	};
	visit(document);
	return texts.slice(0, 20).join("\n");
})()`

var readTopmostSubmitModal = bridge.TopmostModalNodeIDInFrame

// takeASnapshotFirst is the one spelling of the only advice that re-resolves an absent
// target. Every refusal that carries it composes it from here, so the four copies the
// endpoint used to format cannot drift apart again.
const takeASnapshotFirst = "take a /snapshot first"

// ErrTargetNotFound is the one owner of "this target cannot be resolved" — a stale ref
// recovery could not heal, a ref that never existed, a drag destination that is gone. Every
// producer wraps it and the endpoint answers one 404 with the remedy, so the arm keys on the
// sentinel rather than on the wording. Retrying is futile by construction: the target is
// absent, not busy.
var ErrTargetNotFound = errors.New("not found")

// targetNotFound names WHAT is missing ("ref e2", "drag destination e7"). The recovery
// matcher's threshold and score stay out of it — they describe the matcher's internals,
// not anything the caller can act on, and ride in the recovery record the endpoint puts
// in details.
func targetNotFound(subject string) error {
	return fmt.Errorf("%s %w - %s", subject, ErrTargetNotFound, takeASnapshotFirst)
}

func refNotFound(ref string) error {
	return targetNotFound("ref " + ref)
}

func (h *Handlers) targetNotFoundOn(tabID, subject string) error {
	if h.Bridge.GetRefCache(tabID) == nil {
		return fmt.Errorf("%s %w - tab %s has no snapshot - %s", subject, ErrTargetNotFound, tabID, takeASnapshotFirst)
	}
	return targetNotFound(subject)
}

func (h *Handlers) refNotFoundOn(tabID, ref string) error {
	return h.targetNotFoundOn(tabID, "ref "+ref)
}

type submitStateSnapshot struct {
	URL        string `json:"url"`
	DialogOpen bool   `json:"dialogOpen"`

	liveRegions     string
	liveRegionsRead bool
}

type submitInvalidField struct {
	Name    string `json:"name"`
	Label   string `json:"label,omitempty"`
	Message string `json:"message"`
}

type submitPostState struct {
	Status                string               `json:"status"`
	Signal                string               `json:"signal"`
	Dispatch              string               `json:"dispatch"`
	ActionTimedOut        bool                 `json:"actionTimedOut"`
	ElapsedMS             int64                `json:"elapsedMs"`
	LiveRegionText        string               `json:"liveRegionText,omitempty"`
	LiveRegionBlocked     bool                 `json:"liveRegionBlocked,omitempty"`
	LiveRegionBlockReason string               `json:"liveRegionBlockReason,omitempty"`
	LiveRegionWarning     string               `json:"liveRegionWarning,omitempty"`
	InvalidFields         []submitInvalidField `json:"invalidFields,omitempty"`
	Before                submitStateSnapshot  `json:"before,omitzero"`
	After                 submitStateSnapshot  `json:"after,omitzero"`
}

func (h *Handlers) guardLiveRegionText(post *submitPostState) {
	if post.LiveRegionText == "" {
		return
	}
	result := h.ContentGuard.Scan(post.LiveRegionText, post.After.URL)
	if result.Blocked {
		post.LiveRegionText = ""
		post.LiveRegionBlocked = true
		post.LiveRegionBlockReason = result.BlockReason
		return
	}
	post.LiveRegionText = result.Text
	post.LiveRegionWarning = result.Warning
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
	// A failed live-region read leaves the snapshot unread rather than failing it: the
	// frame it runs in can vanish with the very navigation the URL signal reports.
	var liveRegions string
	liveErr := h.Bridge.EvaluateInFrame(ctx, h.selectorFrameID(tabID), submitLiveRegionsJS, &liveRegions, bridge.EvalOpts{})
	return submitStateSnapshot{
		URL:             strings.TrimSpace(currentURL),
		DialogOpen:      dialogOpen,
		liveRegions:     liveRegions,
		liveRegionsRead: liveErr == nil,
	}, nil
}

func newLiveRegionText(before, after submitStateSnapshot) string {
	if !before.liveRegionsRead || !after.liveRegionsRead || after.liveRegions == "" {
		return ""
	}
	known := make(map[string]bool)
	for text := range strings.SplitSeq(before.liveRegions, "\n") {
		known[text] = true
	}
	var fresh []string
	for text := range strings.SplitSeq(after.liveRegions, "\n") {
		if !known[text] {
			fresh = append(fresh, text)
		}
	}
	capped, _ := truncateChars(strings.Join(fresh, "\n"), submitLiveRegionTextLimit)
	return capped
}

func submitTransitionSignal(before, after submitStateSnapshot) (signal, liveRegionText string) {
	liveRegionText = newLiveRegionText(before, after)
	switch {
	case before.URL != after.URL:
		return "url_changed", liveRegionText
	case before.DialogOpen && !after.DialogOpen:
		return "dialog_closed", liveRegionText
	case liveRegionText != "":
		return "live_region_changed", liveRegionText
	}
	return "", ""
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

const submitInvalidWatchDisarmTimeout = 2 * time.Second

func (h *Handlers) armSubmitInvalidWatch(ctx context.Context, nodeID int64, requireSubmitter bool) bool {
	var armed bool
	args := []map[string]any{{"value": requireSubmitter}}
	if err := h.Bridge.CallFunctionOnNode(ctx, nodeID, armSubmitInvalidWatchJS, args, &armed); err != nil {
		return false
	}
	return armed
}

func (h *Handlers) disarmSubmitInvalidWatch(tabCtx context.Context, nodeID int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(tabCtx), submitInvalidWatchDisarmTimeout)
	defer cancel()
	_ = h.Bridge.CallFunctionOnNode(ctx, nodeID, disarmSubmitInvalidWatchJS, nil, nil)
}

func (h *Handlers) readInvalidSubmitFields(ctx context.Context, nodeID int64) []submitInvalidField {
	var fields []submitInvalidField
	if err := h.Bridge.CallFunctionOnNode(ctx, nodeID, submitInvalidFieldsJS, nil, &fields); err != nil {
		return nil
	}
	return fields
}

func (h *Handlers) probeValidationBlocked(ctx context.Context, nodeID int64, dispatch string, actionTimedOut bool, started time.Time) (submitPostState, bool) {
	invalid := h.readInvalidSubmitFields(ctx, nodeID)
	if len(invalid) == 0 {
		return submitPostState{}, false
	}
	return submitPostState{
		Status:         "blocked",
		Signal:         "validation_blocked",
		Dispatch:       dispatch,
		ActionTimedOut: actionTimedOut,
		ElapsedMS:      time.Since(started).Milliseconds(),
		InvalidFields:  invalid,
	}, true
}

func (h *Handlers) settleWaitNavOutcome(ctx context.Context, nodeID int64, result map[string]any) map[string]any {
	if result == nil {
		result = make(map[string]any)
	}
	if result[bridge.ResultNavigated] == true {
		return result
	}
	result[bridge.ResultNavigated] = false
	if nodeID <= 0 {
		return result
	}
	return h.attachValidationBlocked(ctx, nodeID, result)
}

func (h *Handlers) attachValidationBlocked(ctx context.Context, nodeID int64, result map[string]any) map[string]any {
	if result == nil {
		result = make(map[string]any)
	}
	if result[bridge.ResultNavigated] == true {
		return result
	}
	if blocked, ok := h.probeValidationBlocked(ctx, nodeID, "acknowledged", false, time.Now()); ok {
		result["postState"] = blocked
	}
	return result
}

// pollSubmitPostState observes only conservative terminal signals. The caller
// supplies a fresh bounded child of the live tab context, never the action
// context that may already have reached its deadline.
func (h *Handlers) pollSubmitPostState(ctx context.Context, tabID string, nodeID int64, before submitStateSnapshot, dispatch string, actionTimedOut bool) (submitPostState, error) {
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
			if signal, liveRegionText := submitTransitionSignal(before, after); signal != "" {
				post := submitPostState{
					Status:         "succeeded",
					Signal:         signal,
					Dispatch:       dispatch,
					ActionTimedOut: actionTimedOut,
					ElapsedMS:      time.Since(started).Milliseconds(),
					LiveRegionText: liveRegionText,
					Before:         before,
					After:          after,
				}
				h.guardLiveRegionText(&post)
				return post, nil
			}
			if blocked, ok := h.probeValidationBlocked(ctx, nodeID, dispatch, actionTimedOut, started); ok {
				blocked.Before, blocked.After = before, after
				return blocked, nil
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

// ErrStaleSubmitTarget refuses a click whose ref is stale and whose target submits a
// form. The exactly-once rule the submit path already states cannot be honoured by an
// automatic retry, and the DOM — not the request — decides whether a click submits: an
// agent working from a snapshot row `e2:button "Place order"` has no way to declare it.
//
// Submit-detection is a PROXY for non-idempotency, not a solution to it. A delete link is
// as dangerous and the DOM cannot say so; a policy for non-idempotent clicks at large is a
// separate decision and this refusal does not claim to cover the class.
var ErrStaleSubmitTarget = fmt.Errorf("ref is stale and its target submits a form; %s and click the fresh ref", takeASnapshotFirst)

// submitControlJS reports whether the node is the control that submits a form. A var so a
// test can stub it: the guard's own effect is only visible against a real page, and the
// stub is how the pre-fix dispatch is reproduced. A <button>
// inside a form submits by DEFAULT — an absent type attribute means submit — which is the
// case an agent is least likely to have declared.
var submitControlJS = `function() {
  var el = ` + submitControlOfNodeJS + `;
  if (!el || !el.form) return false;
  var type = (el.getAttribute('type') || '').toLowerCase();
  if (el.tagName.toLowerCase() === 'input') return type === 'submit' || type === 'image';
  return type === '' || type === 'submit';
}`

// recoveredNodeSubmits answers whether a click about to be dispatched at a recovered node
// would submit a form. Only clicks are asked: no other action kind dispatches a submit,
// and a read of the DOM per recovery attempt is not free.
//
// A failed read answers false. The alternative — refusing when the page cannot be read —
// would turn every transport hiccup into a refusal on the ordinary stale-link path, which
// is the far more common one.
func (h *Handlers) recoveredNodeSubmits(ctx context.Context, req bridge.ActionRequest, nodeID int64) bool {
	if bridge.CanonicalActionKind(req.Kind) != bridge.ActionClick || nodeID <= 0 {
		return false
	}
	var submits bool
	if err := h.Bridge.CallFunctionOnNode(ctx, nodeID, submitControlJS, nil, &submits); err != nil {
		return false
	}
	return submits
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
	dispatch := h.recoveryDispatch(req, cfg, resolvedTabID, h.refFrameID(resolvedTabID, req.Ref))
	if refMissing {
		if submitClick {
			return nil, "", nil, ErrStaleSubmitTarget
		}
		rr, recRes, recErr := h.Recovery.Attempt(ctx, resolvedTabID, req.Ref, req.Kind, dispatch)
		if recErr != nil {
			// Nothing was dispatched, so there is no recovery record to publish: the
			// refusal happened before the match was used for anything. Absence cannot be
			// misread, where a block reading recovered:true would suggest a click landed.
			if errors.Is(recErr, ErrStaleSubmitTarget) {
				return nil, "", nil, recErr
			}
			return nil, "", &rr, h.refNotFoundOn(resolvedTabID, req.Ref)
		}
		return recRes, "", &rr, nil
	}

	result, backend, err := h.executeAction(ctx, *req, cfg)
	if err != nil && !submitClick && shouldRetryPointerAction(*req, err) {
		if (req.Ref != "" || req.ToSelector != "") && bridge.IsStaleNodeError(err) {
			recordStaleRefRetry()
			h.refreshRefCache(ctx, resolvedTabID)
			if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil && req.Ref != "" {
				if target, ok := cache.Lookup(req.Ref); ok {
					req.NodeID = target.BackendNodeID
				}
			}
		}
		h.refreshActionNodeIDFromSelector(ctx, req)
		if secErr := h.refreshActionSecondaryTargets(ctx, resolvedTabID, req); secErr != nil {
			err = secErr
		} else {
			time.Sleep(pointerRetryDelay)
			result, backend, err = h.executeAction(ctx, *req, cfg)
		}
	}

	var rr *recovery.RecoveryResult
	if err != nil && !submitClick && req.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, req.Ref) {
		r2, recRes, recErr := h.Recovery.AttemptWithClassification(
			ctx, resolvedTabID, req.Ref, req.Kind,
			recovery.ClassifyFailure(err),
			dispatch,
		)
		if errors.Is(recErr, ErrStaleSubmitTarget) {
			return nil, "", nil, recErr
		}
		rr = &r2
		switch {
		case recErr == nil:
			result, err = recRes, nil
		case errors.Is(recErr, errRefFrameReplaced):
			err = fmt.Errorf("%w: %w", h.refNotFoundOn(resolvedTabID, req.Ref), recErr)
		}
	}
	if err != nil && !submitClick && req.Ref != "" && bridge.IsStaleNodeError(err) {
		err = fmt.Errorf("%w: %w", h.refNotFoundOn(resolvedTabID, req.Ref), err)
	}
	return result, backend, rr, err
}

// recoveryDispatch is the one executor both recovery arms hand to the engine. Recovery
// dispatches at a node it MATCHED rather than resolved, so every guard on a matched node
// lives here: a guard added to one arm only is how a cached stale ref came to place an
// order the missing-ref arm refused.
func (h *Handlers) recoveryDispatch(req *bridge.ActionRequest, cfg *config.RuntimeConfig, tabID, mintedIn string) recovery.ActionExecutor {
	return func(ctx context.Context, _ string, nodeID int64) (map[string]any, error) {
		req.NodeID = nodeID
		if h.matchLeftRefFrame(tabID, mintedIn, nodeID) {
			return nil, errRefFrameReplaced
		}
		if h.recoveredNodeSubmits(ctx, *req, nodeID) {
			return nil, ErrStaleSubmitTarget
		}
		if err := h.refreshActionSecondaryTargets(ctx, tabID, req); err != nil {
			return nil, err
		}
		res, _, err := h.executeAction(ctx, *req, cfg)
		return res, err
	}
}

// errRefFrameReplaced refuses a recovery match outside the frame the ref was minted in:
// a replaced iframe is a new document, so a lookalike there is a different field.
var errRefFrameReplaced = errors.New("the frame the ref was in has been replaced")

func (h *Handlers) refFrameID(tabID, ref string) string {
	target, _ := h.Bridge.GetRefCache(tabID).Lookup(ref)
	return target.FrameID
}

func (h *Handlers) matchLeftRefFrame(tabID, mintedIn string, nodeID int64) bool {
	cache := h.Bridge.GetRefCache(tabID)
	if mintedIn == "" || cache == nil {
		return false
	}
	for _, target := range cache.Targets {
		if target.BackendNodeID == nodeID {
			return target.FrameID != mintedIn
		}
	}
	return true
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

	return bridge.IsStaleNodeError(err)
}

// refreshActionSecondaryTargets re-resolves every selector-bearing field the request
// carries BEYOND the primary target, against the current snapshot. The primary is
// re-resolved by the caller (recovery's descriptor match, or the ref-cache lookup on
// the pointer retry); everything else must be re-derived rather than carried over,
// because a node id resolved before a refresh can denote a node the DOM change
// destroyed. Today the only secondary field is the drag destination; a future
// selector-bearing field is re-resolved by adding it HERE, not at each site — the
// census in action_drag_destination_test.go pins that every re-resolution site calls
// this.
//
// A destination ref that no longer resolves is re-found by EXACT identity — the
// unique node in the refreshed snapshot with the intent-cached descriptor's role and
// name — never by the fuzzy heal the source gets. Refs are positional per snapshot,
// so an ordinal re-lookup can denote a different element; and a drop is positional
// too, so a fuzzy lookalike match would land the payload somewhere the caller never
// named while reporting dragged:true. Measured: the default 0.4 match confidence
// happily "recovers" a deleted destination onto its sibling. Whether the source's
// fuzzy heal should report what it did is a separate, pending question; nothing here
// classifies recovery outcomes.
func (h *Handlers) refreshActionSecondaryTargets(ctx context.Context, tabID string, req *bridge.ActionRequest) error {
	toSelector := strings.TrimSpace(req.ToSelector)
	if toSelector == "" {
		return nil
	}
	req.ToNodeID = 0
	if resolution, err := h.resolveActionRequestDestination(ctx, tabID, req); err == nil && !resolution.refMissing && req.ToNodeID != 0 {
		return nil
	}
	req.ToNodeID = 0
	if h.Recovery != nil {
		if sel := selector.Parse(toSelector); sel.Kind == selector.KindRef {
			if entry, ok := h.Recovery.IntentCache.Lookup(tabID, sel.Value); ok {
				if nid, found := uniqueNodeForDescriptor(h.Bridge.GetRefCache(tabID), entry.Descriptor); found {
					req.ToNodeID = nid
					return nil
				}
			}
		}
	}
	return h.targetNotFoundOn(tabID, "drag destination "+toSelector)
}

// uniqueNodeForDescriptor returns the one node matching the descriptor's role and
// name exactly, and refuses when zero or several do — an ambiguous destination is a
// coin flip over where the payload lands.
func uniqueNodeForDescriptor(cache *bridge.RefCache, desc semantic.ElementDescriptor) (int64, bool) {
	if cache == nil || (desc.Role == "" && desc.Name == "") {
		return 0, false
	}
	var match int64
	count := 0
	for i := range cache.Nodes {
		node := cache.Nodes[i]
		if node.Role != desc.Role || node.Name != desc.Name || node.NodeID == 0 {
			continue
		}
		match = node.NodeID
		count++
	}
	if count != 1 {
		return 0, false
	}
	return match, true
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

func (h *Handlers) refreshRefCache(ctx context.Context, tabID string) {
	nodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		return
	}
	flat, _ := bridge.BuildSnapshot(nodes, bridge.FilterInteractive, -1)
	prev := h.Bridge.GetRefCache(tabID)
	if prev != nil {
		bridge.CarryDOMMetadata(prev.Nodes, flat)
	}
	_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, flat)
	h.Bridge.SetRefCache(tabID, bridge.EpochRefs(prev, flat))
}

func publishVocab(w http.ResponseWriter, tabID, vocab string) {
	if vocab == "" {
		return
	}
	if tabID != "" {
		w.Header().Set(activity.HeaderPTTabID, tabID)
	}
	w.Header().Set(vocabHeader, vocab)
}

func (h *Handlers) tabVocab(tabID string) string {
	if cache := h.Bridge.GetRefCache(tabID); cache != nil {
		return cache.DomEpoch
	}
	return ""
}

func (h *Handlers) publishTabVocab(w http.ResponseWriter, tabID string) {
	publishVocab(w, tabID, h.tabVocab(tabID))
}

func (h *Handlers) publishVocabIfReepoched(w http.ResponseWriter, tabID, before string) {
	if vocab := h.tabVocab(tabID); vocab != before {
		publishVocab(w, tabID, vocab)
	}
}

func dialogPendingAfterTimeout(err error, tabID string, b bridge.BridgeAPI) *bridge.DialogState {
	if !errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return pendingTabDialog(b, tabID)
}
