package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// autoSwitchGraceTimeout is a short grace period after a click returns for the
// popup target-created event to reach the browser guard. Popup events normally
// arrive during the click itself; this keeps ordinary clicks from paying a
// long no-popup timeout.
const autoSwitchGraceTimeout = 100 * time.Millisecond

// pendingClickSlot records that a click is in flight for a specific opener tab
// and provides a one-shot channel the popup guard uses to hand the newly
// created target back to the click action.
type pendingClickSlot struct {
	captured chan target.ID
}

// markPendingClick registers an in-flight click for the given opener CDP
// target ID and returns a slot the caller will read from after the click. Any
// existing slot for the same opener is replaced (the previous one is
// abandoned — its receiver will hit its own timeout).
func (tm *TabManager) markPendingClick(openerCDP target.ID) *pendingClickSlot {
	if tm == nil || openerCDP == "" {
		return nil
	}
	slot := &pendingClickSlot{captured: make(chan target.ID, 1)}
	tm.mu.Lock()
	if tm.pendingClicks == nil {
		tm.pendingClicks = make(map[target.ID]*pendingClickSlot)
	}
	tm.pendingClicks[openerCDP] = slot
	tm.mu.Unlock()
	return slot
}

// clearPendingClick removes the slot for openerCDP if it still points at slot.
// It returns any popup target that was already claimed but not yet consumed by
// the action. Idempotent.
func (tm *TabManager) clearPendingClick(openerCDP target.ID, slot *pendingClickSlot) (target.ID, bool) {
	if tm == nil || openerCDP == "" {
		return "", false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	cur, ok := tm.pendingClicks[openerCDP]
	if !ok || (slot != nil && cur != slot) {
		return "", false
	}
	delete(tm.pendingClicks, openerCDP)
	select {
	case targetID := <-cur.captured:
		return targetID, true
	default:
		return "", false
	}
}

// claimPendingPopup is called by the popup guard for every popup target. If a
// click is pending for the opener, the popup is claimed (the guard must not
// close it) and the target ID is delivered on the click's slot. Returns true
// if the popup was claimed.
func (tm *TabManager) claimPendingPopup(openerCDP, newTargetID target.ID) bool {
	if tm == nil || openerCDP == "" {
		return false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	slot := tm.pendingClicks[openerCDP]
	if slot == nil {
		return false
	}
	select {
	case slot.captured <- newTargetID:
		return true
	default:
		// Slot already holds a popup; any second popup from the same click
		// is unexpected — let the guard close it.
		return false
	}
}

// lookupCDPID returns the raw CDP target ID for the given tab ID, if tracked.
func (tm *TabManager) lookupCDPID(tabID string) (target.ID, bool) {
	if tm == nil || tabID == "" {
		return "", false
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry, ok := tm.tabs[tabID]
	if !ok || entry.CDPID == "" {
		return "", false
	}
	return target.ID(entry.CDPID), true
}

// autoSwitchSession tracks a pending-click and turns a captured popup target
// into an adopted, focused tab once the click finishes.
type autoSwitchSession struct {
	tm        *TabManager
	openerCDP target.ID
	slot      *pendingClickSlot

	anchorURL        string
	captureInstalled bool
}

// beginAutoSwitch arms popup-guard cooperation for a click on req.TabID. Returns
// nil if the tab isn't tracked (e.g. unit-test contexts) — callers must
// tolerate a nil session.
func (b *Bridge) beginAutoSwitch(req ActionRequest) *autoSwitchSession {
	if b == nil || b.TabManager == nil || req.TabID == "" {
		return nil
	}
	if req.AutoSwitch != nil && !*req.AutoSwitch {
		return nil
	}
	if !b.browserGuardsActive() {
		return nil
	}
	openerCDP, ok := b.lookupCDPID(req.TabID)
	if !ok {
		return nil
	}
	slot := b.markPendingClick(openerCDP)
	if slot == nil {
		return nil
	}
	return &autoSwitchSession{tm: b.TabManager, openerCDP: openerCDP, slot: slot}
}

func (s *autoSwitchSession) prepareWindowOpenCapture(ctx context.Context) {
	if s == nil || s.captureInstalled {
		return
	}
	const script = `(function() {
		if (window.__pinchtabAutoSwitchOpen && window.__pinchtabAutoSwitchOpen.active) return true;
		var state = {
			active: true,
			original: window.open,
			urls: []
		};
		Object.defineProperty(window, "__pinchtabAutoSwitchOpen", {
			value: state,
			configurable: true
		});
		window.open = function(url, target, features) {
			try {
				var raw = (url == null || url === "") ? "about:blank" : String(url);
				state.urls.push({
					url: new URL(raw, window.location.href).href,
					target: target == null ? "" : String(target)
				});
			} catch (e) {}
			return state.original.apply(this, arguments);
		};
		return true;
	})()`
	var installed bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &installed)); err != nil {
		slog.Debug("auto-switch: window.open capture install failed", "err", err)
		return
	}
	s.captureInstalled = installed
}

func (s *autoSwitchSession) prepareNode(ctx context.Context, backendNodeID int64) {
	if s == nil || backendNodeID <= 0 {
		return
	}
	if s.anchorURL == "" {
		if href, err := popupAnchorURL(ctx, backendNodeID); err == nil {
			s.anchorURL = href
		} else {
			slog.Debug("auto-switch: popup anchor lookup failed", "err", err)
		}
	}
	s.prepareWindowOpenCapture(ctx)
}

func popupAnchorURL(ctx context.Context, backendNodeID int64) (string, error) {
	var resolved json.RawMessage
	if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
		"backendNodeId": backendNodeID,
	}, &resolved); err != nil {
		return "", err
	}
	var obj struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolved, &obj); err != nil {
		return "", err
	}
	if obj.Object.ObjectID == "" {
		return "", nil
	}

	const script = `function() {
		var el = this;
		while (el && el.nodeType === 1) {
			if (el.tagName === "A" && el.hasAttribute("href")) {
				var target = (el.getAttribute("target") || "").trim().toLowerCase();
				if (target === "_blank") return el.href || "";
				return "";
			}
			el = el.parentElement;
		}
		return "";
	}`
	var raw json.RawMessage
	if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
		"functionDeclaration": script,
		"objectId":            obj.Object.ObjectID,
		"returnByValue":       true,
	}, &raw); err != nil {
		return "", err
	}
	var out struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Result.Value), nil
}

func (s *autoSwitchSession) restoreWindowOpenAndFallbackURL(ctx context.Context) string {
	if s == nil || !s.captureInstalled {
		return ""
	}
	const script = `(function() {
		var state = window.__pinchtabAutoSwitchOpen;
		if (!state || !state.active) return "";
		try {
			window.open = state.original;
			delete window.__pinchtabAutoSwitchOpen;
		} catch (e) {}
		for (var i = state.urls.length - 1; i >= 0; i--) {
			var item = state.urls[i] || {};
			var target = String(item.target || "").trim().toLowerCase();
			if (target === "" || target === "_blank") return String(item.url || "");
		}
		return "";
	})()`
	var url string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &url)); err != nil {
		slog.Debug("auto-switch: window.open capture restore failed", "err", err)
		return ""
	}
	return strings.TrimSpace(url)
}

func (s *autoSwitchSession) fallbackPopupURL(ctx context.Context) string {
	if s == nil {
		return ""
	}
	if url := s.restoreWindowOpenAndFallbackURL(ctx); url != "" {
		return url
	}
	return strings.TrimSpace(s.anchorURL)
}

// cancel releases the pending-click slot without adopting any popup. Safe to
// call multiple times.
func (s *autoSwitchSession) cancel(ctx context.Context) {
	if s == nil {
		return
	}
	s.restoreWindowOpenAndFallbackURL(ctx)
	if targetID, ok := s.tm.clearPendingClick(s.openerCDP, s.slot); ok && s.tm.browserCtx != nil {
		s.tm.closePopupTarget(targetID, s.openerCDP, "")
	}
}

// finish waits briefly for a popup target created by the click, adopts it,
// focuses it, and annotates result with "switchedToTab". If no popup arrives
// within autoSwitchGraceTimeout, result is returned unchanged.
func (s *autoSwitchSession) finish(ctx context.Context, result map[string]any) map[string]any {
	if s == nil {
		return result
	}
	cancelUnadopted := func() {
		if targetID, ok := s.tm.clearPendingClick(s.openerCDP, s.slot); ok && s.tm.browserCtx != nil {
			s.tm.closePopupTarget(targetID, s.openerCDP, "")
		}
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), autoSwitchGraceTimeout)
	defer cancel()

	var newTargetID target.ID
	select {
	case newTargetID = <-s.slot.captured:
	case <-waitCtx.Done():
		cancelUnadopted()
		return s.createFallbackTab(ctx, result)
	case <-ctx.Done():
		// Honour the parent context too, but drain the channel non-blockingly
		// in case the popup arrived right at cancellation.
		select {
		case newTargetID = <-s.slot.captured:
		default:
			cancelUnadopted()
			return result
		}
	}
	s.tm.clearPendingClick(s.openerCDP, s.slot)
	s.restoreWindowOpenAndFallbackURL(ctx)

	newTabID, err := s.tm.adoptExistingTarget(newTargetID, true)
	if err != nil {
		slog.Warn("auto-switch: adopt popup failed", "tabId", newTabID, "err", err)
		if s.tm.browserCtx != nil {
			s.tm.closePopupTarget(newTargetID, s.openerCDP, "")
		}
		return result
	}
	if err := s.tm.FocusTab(newTabID); err != nil {
		slog.Warn("auto-switch: focus popup failed", "tabId", newTabID, "err", err)
		return result
	}

	if result == nil {
		result = map[string]any{}
	}
	result["switchedToTab"] = newTabID
	slog.Debug("auto-switch: focused popup", "openerCDP", string(s.openerCDP), "newTabId", newTabID)
	return result
}

func (s *autoSwitchSession) createFallbackTab(ctx context.Context, result map[string]any) map[string]any {
	url := s.fallbackPopupURL(ctx)
	if url == "" {
		return result
	}
	newTabID, _, _, err := s.tm.CreateTab(url)
	if err != nil {
		slog.Warn("auto-switch: fallback popup create failed", "url", url, "err", err)
		return result
	}
	if err := s.tm.FocusTab(newTabID); err != nil {
		slog.Warn("auto-switch: fallback popup focus failed", "tabId", newTabID, "err", err)
		return result
	}
	if result == nil {
		result = map[string]any{}
	}
	result["switchedToTab"] = newTabID
	slog.Debug("auto-switch: focused fallback popup", "newTabId", newTabID, "url", url)
	return result
}
