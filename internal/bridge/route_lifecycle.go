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
	offline      bool
	redirect     *RedirectLimit
}

func (s *tabRouteState) idle() bool {
	return len(s.rules) == 0 && !s.offline && s.redirect == nil
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
	if state == nil {
		state = &tabRouteState{}
		rm.perTab[tabID] = state
	}

	replaced, wasReplaced := RouteRule{}, false
	for i, r := range state.rules {
		if r.Pattern == rule.Pattern {
			replaced, wasReplaced = r, true
			state.rules[i] = rule
			break
		}
	}
	if !wasReplaced {
		if len(state.rules) >= MaxRulesPerTab {
			if state.idle() {
				delete(rm.perTab, tabID)
			}
			rm.mu.Unlock()
			return fmt.Errorf("%w: cap is %d", ErrTooManyRules, MaxRulesPerTab)
		}
		state.rules = append(state.rules, rule)
	}
	claim := rm.claimFetchLocked(ctx, state)
	rm.mu.Unlock()

	if err := rm.enableFetch(ctx, tabID, claim); err != nil {
		rm.rollbackClaim(tabID, claim, func(s *tabRouteState) {
			s.rules = withoutRule(s.rules, rule.Pattern)
			if wasReplaced {
				s.rules = append(s.rules, replaced)
			}
		})
		return err
	}
	return nil
}

func withoutRule(rules []RouteRule, pattern string) []RouteRule {
	kept := rules[:0]
	for _, r := range rules {
		if r.Pattern != pattern {
			kept = append(kept, r)
		}
	}
	return kept
}

// fetchClaim is what one AddRule/SetOffline/ArmRedirectLimit call took under the lock: whether
// it registered the tab's listener and whether it claimed the Fetch enable, so
// the CDP work after unlock and any rollback undo exactly that much.
type fetchClaim struct {
	register  bool
	enable    bool
	listenCtx context.Context
	authFn    func() bool
}

// claimFetchLocked marks the tab's Fetch domain as owned under rm.mu so a
// concurrent same-tab claim sees fetchEnabled=true and does not re-enable.
// The proxy-auth gate is snapshotted here and invoked only after unlock.
func (rm *RouteManager) claimFetchLocked(ctx context.Context, state *tabRouteState) fetchClaim {
	claim := fetchClaim{register: state.listenCancel == nil, enable: !state.fetchEnabled, authFn: rm.proxyAuthActive}
	if claim.register {
		state.listenCtx, state.listenCancel = context.WithCancel(ctx)
	}
	if claim.enable {
		state.fetchEnabled = true
	}
	claim.listenCtx = state.listenCtx
	return claim
}

// enableFetch is the module's one Fetch enabler: it suppresses the proxy-auth
// listener's blanket continue BEFORE dispatch takes over and keeps
// handleAuthRequests on so proxy challenges stay answerable.
func (rm *RouteManager) enableFetch(ctx context.Context, tabID string, claim fetchClaim) error {
	if claim.register {
		rm.registerListener(claim.listenCtx, tabID)
	}
	if !claim.enable {
		return nil
	}
	rm.suppressPause(tabID, true)
	handleAuth := claim.authFn != nil && claim.authFn()
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return fetch.Enable().
			WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}).
			WithHandleAuthRequests(handleAuth).
			Do(c)
	})); err != nil {
		rm.suppressPause(tabID, false)
		return fmt.Errorf("fetch.enable: %w", err)
	}
	return nil
}

// rollbackClaim undoes exactly what one failed claim did: the caller's own
// mutation through undo, the Fetch enable it claimed, and the listener it
// registered. Anything a concurrent caller added meanwhile is left standing,
// because that caller was told nil.
func (rm *RouteManager) rollbackClaim(tabID string, claim fetchClaim, undo func(*tabRouteState)) {
	rm.mu.Lock()
	s := rm.perTab[tabID]
	if s == nil {
		rm.mu.Unlock()
		return
	}
	undo(s)
	if claim.enable {
		s.fetchEnabled = false
	}
	var newCancel context.CancelFunc
	if claim.register {
		newCancel = s.listenCancel
		s.listenCtx, s.listenCancel = nil, nil
	}
	if s.idle() {
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

	release := rm.releaseLocked(tabID, state)
	rm.mu.Unlock()
	release(ctx)
	return removed, nil
}

// releaseLocked drops the tab's state once nothing owns it any more (no rules,
// not offline) and returns the CDP work to run after unlock; while something
// still owns it the returned func is a no-op.
func (rm *RouteManager) releaseLocked(tabID string, state *tabRouteState) func(ctx context.Context) {
	if !state.idle() {
		return func(context.Context) {}
	}
	wasEnabled := state.fetchEnabled
	cancel := state.listenCancel
	state.fetchEnabled = false
	state.listenCancel = nil
	state.listenCtx = nil
	delete(rm.perTab, tabID)
	return func(ctx context.Context) {
		if wasEnabled {
			rm.disableFetch(ctx, tabID)
		} else {
			rm.suppressPause(tabID, false)
		}
		if cancel != nil {
			cancel()
		}
	}
}

// disableFetch hands the Fetch domain back to proxy auth when credentials are
// configured (disabling it would kill auth handling too), otherwise disables
// it. Unsuppress first so no paused request goes unanswered.
func (rm *RouteManager) disableFetch(ctx context.Context, tabID string) {
	if rm.proxyAuthOn() {
		rm.suppressPause(tabID, false)
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return fetch.Enable().WithHandleAuthRequests(true).Do(c)
		})); err != nil {
			slog.Debug("fetch re-enable for proxy auth failed during route teardown", "tabId", tabID, "err", err)
		}
		return
	}
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return fetch.Disable().Do(c)
	})); err != nil {
		slog.Debug("fetch.disable failed during route teardown", "tabId", tabID, "err", err)
	}
	rm.suppressPause(tabID, false)
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
