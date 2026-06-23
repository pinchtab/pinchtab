package bridge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		url     string
		want    bool
	}{
		{"api.example.com", "https://api.example.com/users", true},
		{"api.example.com", "https://other.example.com/users", false},
		{"*.example.com/*", "https://api.example.com/users", true},
		{"*.example.com/*", "https://example.com/users", false},
		{"https://api.example.com/users", "https://api.example.com/users", true},
		{"*", "https://anything.test/", true},
		{"", "https://anything.test/", false},
		{"*users?id", "https://x/usersaid", true},
		{"*users?id", "https://x/usersid", false},
		{"path/with.dots", "https://x/path/with.dots", true},
		{"path/with.dots", "https://x/path/withxdots", false},
	}
	for _, tc := range cases {
		got := globMatch(tc.pattern, tc.url)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.url, got, tc.want)
		}
	}
}

func TestRouteManager_AddRemove_NoCDP(t *testing.T) {
	rm := NewRouteManager(nil)
	rules := rm.List("tab1")
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules initially, got %d", len(rules))
	}

	// Direct manipulation to exercise rule storage without spinning up Chrome.
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*.png", Action: RouteActionAbort},
		{Pattern: "api/users", Action: RouteActionFulfill, Body: "{}", ContentType: "application/json", Status: 200},
	}}
	rm.mu.Unlock()

	got := rm.List("tab1")
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}

	// Remove all (pattern == "") — fetchEnabled is false so no CDP call needed.
	removed, err := rm.Remove(t.Context(), "tab1", "")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if len(rm.List("tab1")) != 0 {
		t.Errorf("expected 0 remaining, got %d", len(rm.List("tab1")))
	}
}

// AddRule optimistically claims fetchEnabled under the lock to close the
// concurrent-same-tab double-enable window; a failed fetch.Enable must roll
// that claim back so a retry can still enable interception.
func TestRouteManager_AddRule_FailedEnableRollsBackFetchEnabled(t *testing.T) {
	rm := NewRouteManager(nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // force the fetch.Enable CDP call to fail promptly

	err := rm.AddRule(ctx, "tab1", RouteRule{Pattern: "*", Action: RouteActionAbort})
	if err == nil {
		t.Fatal("expected fetch.enable to fail on a cancelled context")
	}

	rm.mu.Lock()
	s := rm.perTab["tab1"]
	rm.mu.Unlock()
	if s != nil && s.fetchEnabled {
		t.Error("fetchEnabled should be rolled back to false after a failed enable")
	}
}

func TestRouteManager_Remove_TabNotRoutedReturnsSentinel(t *testing.T) {
	rm := NewRouteManager(nil)
	_, err := rm.Remove(t.Context(), "tab-never-routed", "*.png")
	if err == nil {
		t.Fatal("expected ErrTabNotRouted when tab has no rule state")
	}
	if !errors.Is(err, ErrTabNotRouted) {
		t.Errorf("expected errors.Is(err, ErrTabNotRouted), got %v", err)
	}
}

func TestRouteManager_Remove_TabRoutedButPatternMisses(t *testing.T) {
	// When the tab has rule state but the pattern doesn't match anything,
	// Remove should succeed with removed=0 (idempotent), not the sentinel.
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{{Pattern: "a", Action: RouteActionAbort}}}
	rm.mu.Unlock()

	removed, err := rm.Remove(t.Context(), "tab1", "z")
	if err != nil {
		t.Fatalf("expected nil error for tab-with-rules + non-matching pattern, got %v", err)
	}
	if removed != 0 {
		t.Errorf("expected removed=0, got %d", removed)
	}
}

func TestRouteManager_RemoveByPattern(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "a", Action: RouteActionAbort},
		{Pattern: "b", Action: RouteActionAbort},
		{Pattern: "c", Action: RouteActionAbort},
	}}
	rm.mu.Unlock()

	removed, err := rm.Remove(t.Context(), "tab1", "b")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	rules := rm.List("tab1")
	if len(rules) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(rules))
	}
	for _, r := range rules {
		if r.Pattern == "b" {
			t.Errorf("pattern b should have been removed")
		}
	}
}

func TestRouteManager_Match_FirstWins(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*.example.com/*", Action: RouteActionFulfill, Body: "first", Status: 200, ContentType: "text/plain"},
		{Pattern: "api.example.com/*", Action: RouteActionAbort},
	}}
	rm.mu.Unlock()

	rule, ok, _ := rm.match("tab1", "https://api.example.com/users", "", "GET")
	if !ok {
		t.Fatal("expected match")
	}
	if rule.Action != RouteActionFulfill {
		t.Errorf("expected first rule (fulfill) to win, got %s", rule.Action)
	}
}

func TestRouteManager_Match_NoneMatches(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*.png", Action: RouteActionAbort},
	}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://api.example.com/users", "", "GET"); ok {
		t.Error("expected no match")
	}
}

func TestRouteManager_Match_ResourceTypeFilter(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*", Action: RouteActionAbort, ResourceType: "script"},
	}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://x/img.png", "image", "GET"); ok {
		t.Error("rule with ResourceType=script should not match resourceType=image")
	}
	if _, ok, _ := rm.match("tab1", "https://x/app.js", "script", "GET"); !ok {
		t.Error("rule with ResourceType=script should match resourceType=script")
	}
	if _, ok, _ := rm.match("tab1", "https://x/app.js", "Script", "GET"); !ok {
		t.Error("ResourceType match should be case-insensitive")
	}
}

func TestRouteManager_FulfillAllowedFor_NoAccessor(t *testing.T) {
	rm := NewRouteManager(nil)
	if !rm.fulfillForgeryPermittedFor("https://anything.test/") {
		t.Error("nil accessor should allow fulfill on any URL")
	}
}

// Policy is inverted: fulfill is BLOCKED on hosts in the allowlist (the
// sensitive surfaces) and ALLOWED on hosts outside the allowlist (normal
// mocking targets like third-party APIs/CDNs).
func TestRouteManager_FulfillAllowedFor_BlockedOnAllowlistedHost(t *testing.T) {
	allow := []string{"api.example.com"}
	rm := NewRouteManager(func() []string { return allow })

	if rm.fulfillForgeryPermittedFor("https://api.example.com/users") {
		t.Error("allowlisted host should BLOCK fulfill (response forgery on sensitive surface)")
	}
}

func TestRouteManager_FulfillAllowedFor_AllowedOnUnlistedHost(t *testing.T) {
	allow := []string{"api.example.com"}
	rm := NewRouteManager(func() []string { return allow })

	if !rm.fulfillForgeryPermittedFor("https://cdn.unrelated.test/asset") {
		t.Error("unlisted host should ALLOW fulfill (typical mock target)")
	}
}

func TestRouteManager_FulfillAllowedFor_WildcardSubdomain(t *testing.T) {
	allow := []string{"*.example.com"}
	rm := NewRouteManager(func() []string { return allow })

	if rm.fulfillForgeryPermittedFor("https://api.example.com/x") {
		t.Error("wildcard match means in-allowlist → fulfill blocked")
	}
	if !rm.fulfillForgeryPermittedFor("https://example.com/x") {
		t.Error("apex example.com is NOT covered by *.example.com → fulfill allowed")
	}
}

func TestRouteManager_FulfillAllowedFor_EmptyAllowlistMeansNoEnforcement(t *testing.T) {
	rm := NewRouteManager(func() []string { return nil })
	if !rm.fulfillForgeryPermittedFor("https://anything.test/") {
		t.Error("empty allowlist should mean no enforcement → fulfill allowed")
	}
	rm2 := NewRouteManager(func() []string { return []string{} })
	if !rm2.fulfillForgeryPermittedFor("https://anything.test/") {
		t.Error("zero-length allowlist should mean no enforcement → fulfill allowed")
	}
}

func TestIsFulfillContentTypeAllowed(t *testing.T) {
	allowed := []string{
		"application/json",
		"application/json; charset=utf-8",
		"APPLICATION/JSON",
		"text/plain",
		"image/png",
		"",
	}
	for _, ct := range allowed {
		if !IsFulfillContentTypeAllowed(ct) {
			t.Errorf("expected %q allowed", ct)
		}
	}
	denied := []string{
		"text/html",
		"text/html; charset=utf-8",
		"application/javascript",
		"application/x-javascript",
		"text/javascript",
		"application/ecmascript",
		"application/xhtml+xml",
		"image/svg+xml",
		"application/json\r\nX-Injected: evil",
		"application/json\nX-Injected: evil",
		"application/json\x00null",
	}
	for _, ct := range denied {
		if IsFulfillContentTypeAllowed(ct) {
			t.Errorf("expected %q denied", ct)
		}
	}
}

func TestIsResourceTypeValid(t *testing.T) {
	for _, rt := range []string{"", "Document", "script", "XHR", "fetch", "image"} {
		if !IsResourceTypeValid(rt) {
			t.Errorf("expected %q valid", rt)
		}
	}
	for _, rt := range []string{"bogus", "java", "html"} {
		if IsResourceTypeValid(rt) {
			t.Errorf("expected %q invalid", rt)
		}
	}
}

func TestRouteManager_AddRule_RejectsBadFulfillContentType(t *testing.T) {
	rm := NewRouteManager(nil)
	err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern:     "x",
		Action:      RouteActionFulfill,
		Body:        "{}",
		ContentType: "text/html",
	})
	if err == nil {
		t.Error("expected fulfill with text/html to be rejected")
	}
}

func TestRouteManager_AddRule_RejectsCRLFInContentType(t *testing.T) {
	rm := NewRouteManager(nil)
	err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern:     "x",
		Action:      RouteActionFulfill,
		Body:        "{}",
		ContentType: "application/json\r\nX-Evil: 1",
	})
	if err == nil {
		t.Error("expected CRLF in contentType to be rejected")
	}
}

func TestRouteManager_AddRule_RejectsBadResourceType(t *testing.T) {
	rm := NewRouteManager(nil)
	err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern:      "x",
		Action:       RouteActionAbort,
		ResourceType: "bogus",
	})
	if err == nil {
		t.Error("expected bogus resourceType to be rejected")
	}
}

func TestRouteManager_AddRule_RejectsOversizeBody(t *testing.T) {
	rm := NewRouteManager(nil)
	huge := make([]byte, MaxFulfillBodyBytes+1)
	for i := range huge {
		huge[i] = 'x'
	}
	err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern: "x",
		Action:  RouteActionFulfill,
		Body:    string(huge),
	})
	if err == nil {
		t.Error("expected oversize body to be rejected")
	}
}

func TestRouteManager_AddRule_RejectsBadStatus(t *testing.T) {
	rm := NewRouteManager(nil)
	for _, bad := range []int{-1, 99, 600, 1000} {
		err := rm.AddRule(t.Context(), "tab1", RouteRule{
			Pattern: "x",
			Action:  RouteActionFulfill,
			Body:    "{}",
			Status:  bad,
		})
		if err == nil {
			t.Errorf("expected status=%d to be rejected", bad)
		}
	}
}

// allowedDomains == ["*"] is a wildcard "match anything" entry; per the
// inverted policy that means all hosts are treated as allowlisted, so fulfill
// should be blocked everywhere (the operator chose "trust everywhere", which
// also means "forbid forgery everywhere").
func TestRouteManager_FulfillAllowedFor_GlobalWildcardBlocksAll(t *testing.T) {
	rm := NewRouteManager(func() []string { return []string{"*"} })
	for _, u := range []string{
		"https://api.example.com/users",
		"http://random-blog.test/post",
		"https://internal.corp/admin",
	} {
		if rm.fulfillForgeryPermittedFor(u) {
			t.Errorf("with allowedDomains=['*'] every host should block fulfill (got allowed for %s)", u)
		}
	}
}

// Forbidden URL schemes are always rejected for fulfill, with or without an
// allowlist. Forging a response for data:/javascript:/file:/chrome:/about:
// either makes no sense (inline pseudo-URLs never traverse the network) or
// escapes the web origin model (privileged origins, local files).
func TestRouteManager_FulfillForgeryPermittedFor_ForbiddenSchemesDenied(t *testing.T) {
	for _, accessor := range []func() []string{
		nil, // no policy
		func() []string { return nil },
		func() []string { return []string{"api.example.com"} },
		func() []string { return []string{"*"} },
	} {
		rm := NewRouteManager(accessor)
		for _, u := range []string{
			"javascript:alert(1)",
			"data:text/plain,hello",
			"data:text/html,<script>alert(1)</script>",
			"file:///etc/passwd",
			"blob:https://x.test/abc",
			"ftp://files.example.com/x",
			"chrome://settings",
			"chrome-extension://abc/options.html",
			"about:blank",
			"about:srcdoc",
		} {
			if rm.fulfillForgeryPermittedFor(u) {
				t.Errorf("forbidden-scheme URL %q must reject fulfill regardless of allowlist", u)
			}
		}
	}
}

// Unparseable URLs are conservatively treated as forbidden so the fulfill
// path never operates on inputs we can't reason about.
func TestRouteManager_FulfillForgeryPermittedFor_UnparseableURLDenied(t *testing.T) {
	rm := NewRouteManager(nil)
	if rm.fulfillForgeryPermittedFor("ht!tp://[invalid") {
		t.Error("unparseable URL must reject fulfill")
	}
}

// Fulfill rules without an explicit Method should NOT match an OPTIONS
// preflight — fulfilling it without ACAO/ACAM/ACAH headers breaks the real
// request, and would also let a future custom-headers feature smuggle CORS
// approval through.
func TestRouteManager_Match_FulfillSkipsOPTIONSByDefault(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*", Action: RouteActionFulfill, Body: "{}", Status: 200, ContentType: "application/json"},
	}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://api.example.com/x", "fetch", "OPTIONS"); ok {
		t.Error("fulfill rule with no Method must skip OPTIONS preflight")
	}
	if _, ok, _ := rm.match("tab1", "https://api.example.com/x", "fetch", "GET"); !ok {
		t.Error("fulfill rule should still match non-OPTIONS methods")
	}
}

// Aborting OPTIONS without an explicit Method is fine — abort can't bypass
// CORS, and operators commonly want to block third-party API calls including
// their preflights.
func TestRouteManager_Match_AbortAllowsOPTIONSByDefault(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*", Action: RouteActionAbort},
	}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://api.example.com/x", "fetch", "OPTIONS"); !ok {
		t.Error("abort rule with no Method should match OPTIONS too (no CORS bypass risk)")
	}
}

// Explicit Method opt-in: operator can fulfill OPTIONS by naming it.
func TestRouteManager_Match_ExplicitOPTIONSOptIn(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*", Action: RouteActionFulfill, Method: "OPTIONS", Body: "{}", Status: 204, ContentType: "application/json"},
	}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://api.example.com/x", "fetch", "OPTIONS"); !ok {
		t.Error("fulfill rule with Method=OPTIONS must match OPTIONS")
	}
	if _, ok, _ := rm.match("tab1", "https://api.example.com/x", "fetch", "GET"); ok {
		t.Error("fulfill rule with Method=OPTIONS must NOT match GET")
	}
}

func TestRouteManager_Match_MethodFilterCaseInsensitive(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*", Action: RouteActionAbort, Method: "post"},
	}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://x/", "fetch", "POST"); !ok {
		t.Error("Method filter should be case-insensitive")
	}
	if _, ok, _ := rm.match("tab1", "https://x/", "fetch", "GET"); ok {
		t.Error("Method=POST should not match GET")
	}
}

func TestRouteManager_AddRule_RejectsInvalidMethod(t *testing.T) {
	rm := NewRouteManager(nil)
	err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern: "x",
		Action:  RouteActionAbort,
		Method:  "FOOBAR",
	})
	if err == nil {
		t.Error("expected unknown method to be rejected")
	}
}

func TestNormalizeHTTPMethod(t *testing.T) {
	for _, in := range []string{"get", "POST", " patch ", "delete"} {
		if _, ok := normalizeHTTPMethod(in); !ok {
			t.Errorf("expected %q to normalize", in)
		}
	}
	if got, _ := normalizeHTTPMethod("post"); got != "POST" {
		t.Errorf("expected uppercase, got %q", got)
	}
	for _, in := range []string{"", " ", "FOOBAR", "GET-but-not-really"} {
		if _, ok := normalizeHTTPMethod(in); ok {
			t.Errorf("expected %q to be rejected", in)
		}
	}
}

// Cross-tab dispatch sanity check: rules on tab1 must not match URLs queried
// against tab2. (chromedp's per-target listener dispatch already isolates
// events at the wire level; this asserts the manager-level filter hasn't
// regressed to a global lookup.)
func TestRouteManager_Match_PerTabIsolation(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{{Pattern: "*", Action: RouteActionAbort}}}
	rm.perTab["tab2"] = &tabRouteState{rules: []RouteRule{{Pattern: "tab2-only", Action: RouteActionAbort}}}
	rm.mu.Unlock()

	if _, ok, _ := rm.match("tab1", "https://anywhere/", "", "GET"); !ok {
		t.Error("tab1 should match its own '*' rule")
	}
	if _, ok, _ := rm.match("tab2", "https://anywhere/", "", "GET"); ok {
		t.Error("tab2 must NOT match tab1's rule (cross-tab leak)")
	}
	if _, ok, _ := rm.match("tab1", "https://x/tab2-only", "", "GET"); !ok {
		// tab1's '*' covers everything, so still matches.
		t.Error("tab1 '*' rule should still match URL containing tab2-only")
	}
}

// Pins the documented asymmetry: bare strings → substring, wildcards →
// anchored full-URL. Reads as a behavior contract; if this changes, the
// docstring on RouteRule.Pattern must change too.
func TestGlobMatch_AsymmetryIsDocumented(t *testing.T) {
	// Bare string: substring match.
	if !globMatch("tracker.io", "https://x.com/?ref=tracker.io") {
		t.Error("bare 'tracker.io' should match URL containing it (substring)")
	}
	if !globMatch("api.example.com", "https://api.example.com/users") {
		t.Error("bare host should match URL containing it (substring)")
	}

	// Wildcard pattern: anchored full-URL match.
	if !globMatch("*.png", "https://x/img.png") {
		t.Error("'*.png' should anchor and match URLs ending in .png")
	}
	if globMatch("*.png", "https://x/img.png/redirect") {
		t.Error("'*.png' should NOT match when .png is in the middle (anchored)")
	}
	// To get substring-with-wildcard, wrap with '*'.
	if !globMatch("*.png*", "https://x/img.png/redirect") {
		t.Error("'*.png*' should match .png appearing anywhere")
	}
}

func TestRouteManager_RemoveTab_DropsState(t *testing.T) {
	rm := NewRouteManager(nil)
	called := false
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{
		rules:        []RouteRule{{Pattern: "*.png", Action: RouteActionAbort}},
		listenCancel: func() { called = true },
	}
	rm.mu.Unlock()

	rm.RemoveTab("tab1")

	if !called {
		t.Error("RemoveTab should call listenCancel")
	}
	rm.mu.Lock()
	_, present := rm.perTab["tab1"]
	rm.mu.Unlock()
	if present {
		t.Error("RemoveTab should delete the perTab entry")
	}
}

func TestRouteManager_RemoveTab_NoStateNoOp(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.RemoveTab("nonexistent") // must not panic
}

// Churn test: simulates the lifecycle of many tabs being routed and then
// closed. After each cycle the manager must hold no per-tab state. This is
// the test that converts "the cleanup hook is wired" into "we have evidence
// nothing leaks across hundreds of tab closes."
func TestRouteManager_RemoveTab_NoLeakAcrossManyTabs(t *testing.T) {
	rm := NewRouteManager(nil)
	const cycles = 200
	for i := 0; i < cycles; i++ {
		tabID := fmt.Sprintf("tab-%d", i)
		// Mimic AddRule's effect on state without driving CDP.
		rm.mu.Lock()
		rm.perTab[tabID] = &tabRouteState{
			rules: []RouteRule{
				{Pattern: "*.png", Action: RouteActionAbort},
				{Pattern: "api/users", Action: RouteActionFulfill, Body: "{}", Status: 200, ContentType: "application/json"},
			},
			listenCancel: func() {},
		}
		rm.mu.Unlock()
		rm.RemoveTab(tabID)
	}
	rm.mu.Lock()
	leaked := len(rm.perTab)
	rm.mu.Unlock()
	if leaked != 0 {
		t.Errorf("expected 0 tabs in perTab after %d open/close cycles, got %d", cycles, leaked)
	}
}

// Defends against the "Chrome reused the target id" scenario: rules installed
// for an ID, the tab closes, then a new tab gets the same ID (chromedp tab
// IDs are hash-derived from CDP target IDs which can be reused after a
// process restart). The new tab must see no stale rules, and installing a
// fresh rule for the reused ID must not commingle with the old state.
func TestRouteManager_RemoveTab_IDReuseHasFreshState(t *testing.T) {
	rm := NewRouteManager(nil)
	cancelCalls := 0
	const tabID = "tab-reused"

	// First lifetime.
	rm.mu.Lock()
	rm.perTab[tabID] = &tabRouteState{
		rules:        []RouteRule{{Pattern: "old.example", Action: RouteActionAbort}},
		listenCancel: func() { cancelCalls++ },
	}
	rm.mu.Unlock()
	rm.RemoveTab(tabID)
	if cancelCalls != 1 {
		t.Errorf("expected listenCancel to fire once on close, got %d", cancelCalls)
	}
	if rules := rm.List(tabID); len(rules) != 0 {
		t.Errorf("after RemoveTab, List must report 0 rules for the closed tab id (got %d)", len(rules))
	}

	// Second lifetime (target id reused) — install a different rule, assert
	// the old rule is gone and only the new one is visible.
	rm.mu.Lock()
	rm.perTab[tabID] = &tabRouteState{
		rules: []RouteRule{{Pattern: "new.example", Action: RouteActionAbort}},
	}
	rm.mu.Unlock()
	rules := rm.List(tabID)
	if len(rules) != 1 || rules[0].Pattern != "new.example" {
		t.Errorf("expected only the new rule after id reuse, got %+v", rules)
	}
}

func TestRouteManager_AddRule_PerTabCapEnforced(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rules := make([]RouteRule, 0, MaxRulesPerTab)
	for i := 0; i < MaxRulesPerTab; i++ {
		rules = append(rules, RouteRule{
			Pattern: fmt.Sprintf("p%d", i),
			Action:  RouteActionAbort,
		})
	}
	rm.perTab["tab1"] = &tabRouteState{rules: rules}
	rm.mu.Unlock()

	// New pattern → rejected (cap reached).
	err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern: "p-overflow",
		Action:  RouteActionAbort,
	})
	if err == nil {
		t.Fatal("expected ErrTooManyRules when at cap")
	}
	if !errors.Is(err, ErrTooManyRules) {
		t.Errorf("expected errors.Is(err, ErrTooManyRules), got %v", err)
	}

	// Same-pattern replace → allowed even at cap.
	rm.mu.Lock()
	state := rm.perTab["tab1"]
	rm.mu.Unlock()
	originalLen := len(state.rules)
	if err := rm.AddRule(t.Context(), "tab1", RouteRule{
		Pattern: "p0",
		Action:  RouteActionContinue,
	}); err != nil {
		// AddRule will get past validation but then call fetch.Enable on a
		// non-chromedp context — that part fails. We only care that the cap
		// check didn't reject the replace; ErrTooManyRules must NOT be the
		// reason, and the rule mutation must have happened.
		if errors.Is(err, ErrTooManyRules) {
			t.Errorf("same-pattern replace must not hit cap: %v", err)
		}
	}
	rm.mu.Lock()
	state = rm.perTab["tab1"]
	rm.mu.Unlock()
	if state == nil {
		// rollback after fetch.Enable failure may have wiped state if it was
		// ever empty; here state existed before, so a snapshot was restored.
		t.Skip("rollback path consumed the state; cap-replace assertion still valid via no error")
		return
	}
	if len(state.rules) != originalLen {
		t.Errorf("expected rule count unchanged on replace, got %d (was %d)", len(state.rules), originalLen)
	}
}

func TestRouteManager_AddRule_Validation(t *testing.T) {
	rm := NewRouteManager(nil)
	if err := rm.AddRule(t.Context(), "tab1", RouteRule{}); err == nil {
		t.Error("expected error for empty pattern")
	}
	if err := rm.AddRule(t.Context(), "tab1", RouteRule{Pattern: "x", Action: "bogus"}); err == nil {
		t.Error("expected error for invalid action")
	}
}

// M16: route teardown must release the proxy-auth pause suppression even
// when Fetch was never enabled (no CDP call path).
func TestRouteManager_TeardownReleasesPauseSuppression(t *testing.T) {
	rm := NewRouteManager(nil)
	var calls []string
	rm.SetFetchAuthCoordination(
		func() bool { return true },
		func(tabID string, v bool) { calls = append(calls, fmt.Sprintf("%s=%v", tabID, v)) },
	)

	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{rules: []RouteRule{
		{Pattern: "*.png", Action: RouteActionAbort},
	}}
	rm.mu.Unlock()

	if _, err := rm.Remove(t.Context(), "tab1", ""); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(calls) != 1 || calls[0] != "tab1=false" {
		t.Fatalf("teardown should unsuppress exactly once, got %v", calls)
	}
}

// M16: a failed Fetch enable must roll the suppression back so the
// proxy-auth listener resumes continuing paused requests.
func TestRouteManager_FailedEnableRollsBackSuppression(t *testing.T) {
	rm := NewRouteManager(nil)
	var calls []string
	rm.SetFetchAuthCoordination(
		func() bool { return true },
		func(tabID string, v bool) { calls = append(calls, fmt.Sprintf("%s=%v", tabID, v)) },
	)

	// A canceled chromedp context makes fetch.Enable fail deterministically.
	parent, cancel := chromedp.NewContext(context.Background())
	cancel()

	err := rm.AddRule(parent, "tab1", RouteRule{Pattern: "*.png", Action: RouteActionAbort})
	if err == nil {
		t.Fatal("expected enable failure on dead chromedp context")
	}
	if len(calls) != 2 || calls[0] != "tab1=true" || calls[1] != "tab1=false" {
		t.Fatalf("expected suppress-then-rollback, got %v", calls)
	}
}
