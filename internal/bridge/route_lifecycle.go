package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// RouteManager tracks active per-tab interception rules. It enables CDP fetch
// interception lazily when the first rule is added and disables it when the
// last rule is removed.
type RouteManager struct {
	mu               sync.Mutex
	perTab           map[string]*tabRouteState
	allowedDomainsFn func() []string // nil ⇒ no allowlist enforcement

	// Fetch-domain coordination with proxy auth (see Bridge.tabSetup): while
	// rules own a tab's Fetch domain, the proxy-auth listener's blanket
	// ContinueRequest is suppressed and the route enable must keep
	// handleAuthRequests so challenges stay answerable.
	proxyAuthActive    func() bool                // nil ⇒ no proxy auth configured
	setPauseSuppressed func(tabID string, v bool) // nil ⇒ no coordination
}

// tabRouteState holds per-tab interception state. listenCtx, when non-nil, is
// derived from the tab's chromedp context and gates the listener; cancelling
// listenCancel stops dispatch (chromedp skips listeners with a done context)
// and is called when the last rule is removed or the tab closes.
type tabRouteState struct {
	rules        []RouteRule
	listenCtx    context.Context
	listenCancel context.CancelFunc
	fetchEnabled bool
}

// NewRouteManager constructs a RouteManager. allowedDomainsFn, when non-nil, is
// called at fulfill-time to decide whether the matched URL host is allowed to
// receive a fabricated response body (security.allowedDomains boundary).
// Pass nil to disable that check.
func NewRouteManager(allowedDomainsFn func() []string) *RouteManager {
	return &RouteManager{perTab: make(map[string]*tabRouteState), allowedDomainsFn: allowedDomainsFn}
}

// SetFetchAuthCoordination wires the proxy-auth coordination callbacks: the
// gate reporting whether proxy credentials are configured, and the per-tab
// pause-suppression setter on the owning Bridge.
func (rm *RouteManager) SetFetchAuthCoordination(proxyAuthActive func() bool, setPauseSuppressed func(tabID string, v bool)) {
	if rm == nil {
		return
	}
	rm.proxyAuthActive = proxyAuthActive
	rm.setPauseSuppressed = setPauseSuppressed
}

func (rm *RouteManager) proxyAuthOn() bool {
	return rm.proxyAuthActive != nil && rm.proxyAuthActive()
}

func (rm *RouteManager) suppressPause(tabID string, v bool) {
	if rm.setPauseSuppressed != nil {
		rm.setPauseSuppressed(tabID, v)
	}
}

// MaxFulfillBodyBytes caps the size of a fulfill rule's response body. The
// HTTP handler enforces this for early 400s; AddRule re-checks so non-HTTP
// callers (direct bridge users, future ipc) also benefit.
const MaxFulfillBodyBytes = 1 << 20 // 1 MiB

// MaxRulesPerTab caps how many interception rules a single tab may carry.
// Without a cap an attacker (or runaway agent) could install thousands of
// rules to consume memory and slow every paused request through a long
// per-event match loop. The number is generous for legitimate use — typical
// fixtures have a few rules — and conservative against abuse.
const MaxRulesPerTab = 100

// ErrTooManyRules is returned by AddRule when a tab already holds
// MaxRulesPerTab rules and the incoming rule is not a same-pattern replace.
var ErrTooManyRules = errors.New("too many interception rules on this tab")

// ErrTabNotRouted is returned by Remove when the tab has no rule state
// registered with the manager — distinct from "tab found, pattern matched
// nothing" which returns (0, nil). Callers can map this to a 404 to
// differentiate from a benign no-op removal.
var ErrTabNotRouted = errors.New("tab has no interception rules registered")

// AddRule installs (or replaces, by Pattern) a rule for the given tab.
func (rm *RouteManager) AddRule(ctx context.Context, tabID string, rule RouteRule) error {
	if rm == nil {
		return fmt.Errorf("route manager not initialized")
	}
	if rule.Pattern == "" {
		return fmt.Errorf("pattern required")
	}
	if rule.Action == "" {
		rule.Action = RouteActionContinue
	}
	switch rule.Action {
	case RouteActionContinue, RouteActionAbort, RouteActionFulfill:
	default:
		return fmt.Errorf("invalid action %q", rule.Action)
	}
	if !IsResourceTypeValid(rule.ResourceType) {
		return fmt.Errorf("invalid resourceType %q", rule.ResourceType)
	}
	if rule.Method != "" {
		normalized, ok := normalizeHTTPMethod(rule.Method)
		if !ok {
			return fmt.Errorf("invalid method %q", rule.Method)
		}
		rule.Method = normalized
	}
	// Body cap, status range, and content-type validation apply regardless of
	// Action so non-HTTP callers (MCP, future ipc) can't smuggle malformed
	// values past the bridge by setting Action=abort/continue. Defaults
	// (Status=200, ContentType=application/json) are still applied only when
	// the rule actually uses them (fulfill).
	if len(rule.Body) > MaxFulfillBodyBytes {
		return fmt.Errorf("body exceeds %d bytes (cap)", MaxFulfillBodyBytes)
	}
	if rule.Status != 0 && (rule.Status < 100 || rule.Status > 599) {
		return fmt.Errorf("status %d out of HTTP range (100-599)", rule.Status)
	}
	if rule.ContentType != "" && !IsFulfillContentTypeAllowed(rule.ContentType) {
		return fmt.Errorf("contentType %q is not on the fulfill safe-list (or contains control chars)", rule.ContentType)
	}
	if rule.Action == RouteActionFulfill {
		if rule.Status == 0 {
			rule.Status = 200
		}
		if rule.ContentType == "" {
			rule.ContentType = "application/json"
		}
	}

	compiled, err := compileRulePattern(rule.Pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", rule.Pattern, err)
	}
	rule.compiled = compiled

	rm.mu.Lock()
	state := rm.perTab[tabID]
	isNewState := state == nil
	if state == nil {
		state = &tabRouteState{}
		rm.perTab[tabID] = state
	}

	// Snapshot for rollback on fetch.Enable failure.
	priorRules := append([]RouteRule(nil), state.rules...)
	priorListenCtx := state.listenCtx
	priorListenCancel := state.listenCancel
	priorFetchEnabled := state.fetchEnabled

	replaced := false
	for i, r := range state.rules {
		if r.Pattern == rule.Pattern {
			state.rules[i] = rule
			replaced = true
			break
		}
	}
	if !replaced {
		if len(state.rules) >= MaxRulesPerTab {
			if isNewState {
				delete(rm.perTab, tabID)
			}
			rm.mu.Unlock()
			return fmt.Errorf("%w: cap is %d", ErrTooManyRules, MaxRulesPerTab)
		}
		state.rules = append(state.rules, rule)
	}

	needRegister := state.listenCancel == nil
	needEnable := !state.fetchEnabled
	// Snapshot the (set-once) proxy-auth gate under the lock; invoked after unlock
	// so the callback never runs while holding rm.mu.
	authFn := rm.proxyAuthActive
	if needRegister {
		state.listenCtx, state.listenCancel = context.WithCancel(ctx)
	}
	if needEnable {
		// Claim the enable under the lock so a concurrent same-tab AddRule sees
		// fetchEnabled=true and won't redundantly re-enable Fetch. Rolled back
		// below (via rollbackAddRule) if fetch.Enable fails.
		state.fetchEnabled = true
	}
	listenCtx := state.listenCtx
	rm.mu.Unlock()

	if needRegister {
		rm.registerListener(listenCtx, tabID)
	}
	if needEnable {
		// Suppress the proxy-auth listener's blanket continue BEFORE rules
		// take over dispatch, and keep handleAuthRequests on so proxy
		// challenges stay answerable while routes own the Fetch domain.
		rm.suppressPause(tabID, true)
		handleAuth := authFn != nil && authFn()
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return fetch.Enable().
				WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}).
				WithHandleAuthRequests(handleAuth).
				Do(c)
		})); err != nil {
			rm.suppressPause(tabID, false)
			rm.rollbackAddRule(tabID, isNewState, needRegister, priorRules, priorListenCtx, priorListenCancel, priorFetchEnabled)
			return fmt.Errorf("fetch.enable: %w", err)
		}
	}
	return nil
}

// rollbackAddRule restores the per-tab state captured before a failed AddRule.
// If we registered a fresh listener for this call, its context is cancelled so
// the no-op listener handle is released.
func (rm *RouteManager) rollbackAddRule(tabID string, isNewState, registeredListener bool, priorRules []RouteRule, priorListenCtx context.Context, priorListenCancel context.CancelFunc, priorFetchEnabled bool) {
	rm.mu.Lock()
	s := rm.perTab[tabID]
	if s == nil {
		rm.mu.Unlock()
		return
	}
	var newCancel context.CancelFunc
	if registeredListener {
		newCancel = s.listenCancel
		s.listenCtx = priorListenCtx
		s.listenCancel = priorListenCancel
	}
	s.rules = priorRules
	s.fetchEnabled = priorFetchEnabled
	if isNewState && len(s.rules) == 0 {
		delete(rm.perTab, tabID)
	}
	rm.mu.Unlock()

	if newCancel != nil {
		newCancel()
	}
}

// Remove deletes rules matching pattern. Empty pattern removes all rules for
// the tab. Returns the number of rules removed. When the last rule is removed,
// the listener context is cancelled and CDP fetch interception is disabled.
func (rm *RouteManager) Remove(ctx context.Context, tabID string, pattern string) (int, error) {
	if rm == nil {
		return 0, fmt.Errorf("route manager not initialized")
	}

	rm.mu.Lock()
	state := rm.perTab[tabID]
	if state == nil {
		rm.mu.Unlock()
		return 0, ErrTabNotRouted
	}

	removed := 0
	if pattern == "" {
		removed = len(state.rules)
		state.rules = nil
	} else {
		kept := state.rules[:0]
		for _, r := range state.rules {
			if r.Pattern == pattern {
				removed++
				continue
			}
			kept = append(kept, r)
		}
		state.rules = kept
	}

	teardown := len(state.rules) == 0
	wasEnabled := state.fetchEnabled
	cancel := state.listenCancel
	if teardown {
		state.fetchEnabled = false
		state.listenCancel = nil
		state.listenCtx = nil
		delete(rm.perTab, tabID)
	}
	rm.mu.Unlock()

	if teardown {
		if wasEnabled {
			if rm.proxyAuthOn() {
				// Hand the Fetch domain back to proxy auth instead of
				// disabling it (which would kill auth handling too).
				// Unsuppress first so no paused request goes unanswered.
				rm.suppressPause(tabID, false)
				if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
					return fetch.Enable().WithHandleAuthRequests(true).Do(c)
				})); err != nil {
					slog.Debug("fetch re-enable for proxy auth failed during route teardown", "tabId", tabID, "err", err)
				}
			} else {
				if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
					return fetch.Disable().Do(c)
				})); err != nil {
					slog.Debug("fetch.disable failed during route teardown", "tabId", tabID, "err", err)
				}
				rm.suppressPause(tabID, false)
			}
		} else {
			rm.suppressPause(tabID, false)
		}
		if cancel != nil {
			cancel()
		}
	}
	return removed, nil
}

// RemoveTab drops all rule state for a tab without issuing CDP calls. It is
// the cleanup hook fired by TabManager when a tab closes (manual close,
// eviction, auto-close, or Chrome reporting the target gone). When the tab's
// chromedp context is already canceled, fetch.Disable would fail anyway —
// canceling the listener context is the only meaningful step. Without this
// hook, perTab[tabID] and its cancel func would leak; if Chrome ever reused
// the target id, a stale entry would be found.
func (rm *RouteManager) RemoveTab(tabID string) {
	if rm == nil || tabID == "" {
		return
	}
	rm.mu.Lock()
	state := rm.perTab[tabID]
	if state == nil {
		rm.mu.Unlock()
		return
	}
	cancel := state.listenCancel
	delete(rm.perTab, tabID)
	rm.mu.Unlock()
	// Hand pause dispatch back even though the Bridge drops the whole flag in
	// its own onTabRemoved hook right after — RemoveTab must stay correct on
	// its own, not by courtesy of the caller's cleanup ordering.
	rm.suppressPause(tabID, false)
	if cancel != nil {
		cancel()
	}
}

func (rm *RouteManager) List(tabID string) []RouteRule {
	if rm == nil {
		return nil
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	state := rm.perTab[tabID]
	if state == nil {
		return nil
	}
	out := make([]RouteRule, len(state.rules))
	copy(out, state.rules)
	return out
}
