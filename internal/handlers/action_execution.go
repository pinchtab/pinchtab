package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
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

const takeASnapshotFirst = "take a /snapshot first"

var ErrTargetNotFound = errors.New("not found")

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
	// The live-region frame can vanish with the very navigation the URL signal reports, so a failed read is tolerated.
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
	// A /find entry carries a Query; a descriptor-only entry must not replace it.
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

var ErrStaleSubmitTarget = fmt.Errorf("ref is stale and its target submits a form; %s and click the fresh ref", takeASnapshotFirst)

var submitControlJS = `function() {
  var el = ` + submitControlOfNodeJS + `;
  if (!el || !el.form) return false;
  var type = (el.getAttribute('type') || '').toLowerCase();
  if (el.tagName.toLowerCase() === 'input') return type === 'submit' || type === 'image';
  return type === '' || type === 'submit';
}`

// A failed read answers false: refusing on a transport hiccup would break the far more common stale-link path.
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

func (h *Handlers) missingRefFailure(tabID string) recovery.FailureType {
	if h.Bridge.GetRefCache(tabID) == nil {
		return recovery.FailureNavigation
	}
	return recovery.FailureUnknown
}

func (h *Handlers) executeActionResilient(ctx context.Context, req *bridge.ActionRequest, primarySelector string, cfg *config.RuntimeConfig, resolvedTabID string, refMissing bool) (map[string]any, string, *recovery.RecoveryResult, error) {
	submitClick := bridge.IsSubmitClick(req.Kind, *req)
	dispatch := h.recoveryDispatch(req, cfg, resolvedTabID, h.refFrameID(resolvedTabID, req.Ref))
	if refMissing {
		if submitClick {
			return nil, "", nil, ErrStaleSubmitTarget
		}
		rr, recRes, recErr := h.Recovery.AttemptWithClassification(ctx, resolvedTabID, req.Ref, req.Kind, h.missingRefFailure(resolvedTabID), dispatch)
		if recErr != nil {
			if recoveryRefused(recErr) {
				return nil, "", nil, recErr
			}
			return nil, "", &rr, h.refNotFoundOn(resolvedTabID, req.Ref)
		}
		return recRes, "", &rr, nil
	}

	result, backend, err := h.executeAction(ctx, *req, cfg)
	if err != nil && !submitClick && shouldRetryPointerAction(*req, err) {
		stale := (req.Ref != "" || req.ToSelector != "") && bridge.IsStaleNodeError(err)
		if stale {
			recordStaleRefRetry()
			h.refreshRefCache(ctx, resolvedTabID)
		}
		if !stale || !onlyRecoveryCanRepoint(*req) {
			if primaryErr := h.reresolveStalePrimary(ctx, resolvedTabID, primarySelector, req, err); primaryErr != nil {
				err = primaryErr
			} else if secErr := h.refreshActionSecondaryTargets(ctx, resolvedTabID, req); secErr != nil {
				err = secErr
			} else if waitFor(ctx, pointerRetryDelay) == nil {
				result, backend, err = h.executeAction(ctx, *req, cfg)
			}
		}
	}

	var rr *recovery.RecoveryResult
	if err != nil && !submitClick && req.Ref != "" && h.Recovery != nil && h.Recovery.ShouldAttempt(err, req.Ref) {
		r2, recRes, recErr := h.Recovery.AttemptWithClassification(
			ctx, resolvedTabID, req.Ref, req.Kind,
			recovery.ClassifyFailure(err),
			dispatch,
		)
		if recoveryRefused(recErr) {
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

func (h *Handlers) recoveryDispatch(req *bridge.ActionRequest, cfg *config.RuntimeConfig, tabID, mintedIn string) recovery.ActionExecutor {
	return func(ctx context.Context, _ string, nodeID int64) (map[string]any, error) {
		req.NodeID = nodeID
		if h.matchLeftRefFrame(tabID, mintedIn, nodeID) {
			return nil, errRefFrameReplaced
		}
		if h.recoveredNodeSubmits(ctx, *req, nodeID) {
			return nil, ErrStaleSubmitTarget
		}
		if err := h.refuseTiedRecovery(tabID, req.Ref, nodeID); err != nil {
			return nil, err
		}
		if err := h.refreshActionSecondaryTargets(ctx, tabID, req); err != nil {
			return nil, err
		}
		res, _, err := h.executeAction(ctx, *req, cfg)
		return res, err
	}
}

var errRefFrameReplaced = errors.New("the frame the ref was in has been replaced")

var errRecoveryTied = errors.New("recovery match is tied")

func recoveryRefused(err error) bool {
	return errors.Is(err, ErrStaleSubmitTarget) || errors.Is(err, errRecoveryTied)
}

func (h *Handlers) refuseTiedRecovery(tabID, staleRef string, nodeID int64) error {
	nodes := h.resolveSnapshotNodes(tabID)
	candidate := slices.IndexFunc(nodes, func(n bridge.A11yNode) bool { return n.NodeID == nodeID })
	if candidate < 0 {
		return nil
	}
	tied := twinRefs(semanticDescriptorsFromNodes(nodes), candidate)
	if len(tied) < 2 {
		return nil
	}
	return fmt.Errorf("ref %s %w - %w: refs %s share its role and name; %s and pick one",
		staleRef, ErrTargetNotFound, errRecoveryTied, strings.Join(tied, ", "), takeASnapshotFirst)
}

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

func onlyRecoveryCanRepoint(req bridge.ActionRequest) bool {
	return req.Ref != "" && strings.TrimSpace(req.ToSelector) == ""
}

func (h *Handlers) reresolveStalePrimary(ctx context.Context, tabID, primarySelector string, req *bridge.ActionRequest, dispatchErr error) error {
	if !bridge.IsStaleNodeError(dispatchErr) || primarySelector == "" || selector.Parse(primarySelector).Kind == selector.KindRef {
		return nil
	}
	primary := bridge.ActionRequest{Kind: req.Kind, TabID: req.TabID, Selector: primarySelector}
	if _, err := h.resolveActionRequestSelector(ctx, tabID, &primary); err != nil {
		return fmt.Errorf("re-resolve stale target: %w", err)
	}
	req.NodeID = primary.NodeID
	return nil
}

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

// Several nodes matching the descriptor would make the drop a coin flip, so that case refuses like zero.
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
