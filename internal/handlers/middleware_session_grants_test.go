package handlers

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
)

// grantRoutes names, for every grant, one route inside its group. The table is the
// per-grant coverage the middleware never had: eleven matchers existed and one was
// exercised, because nothing could set the other ten.
var grantRoutes = map[string]struct {
	method string
	path   string
}{
	session.GrantBrowse:    {http.MethodGet, "/snapshot"},
	session.GrantNetwork:   {http.MethodGet, "/network"},
	session.GrantMedia:     {http.MethodGet, "/pdf"},
	session.GrantCookies:   {http.MethodGet, "/cookies"},
	session.GrantClipboard: {http.MethodGet, "/clipboard/read"},
	session.GrantEvaluate:  {http.MethodPost, "/evaluate"},
	session.GrantStorage:   {http.MethodGet, "/storage"},
	session.GrantConsole:   {http.MethodGet, "/console"},
	session.GrantSolve:     {http.MethodGet, "/solvers"},
	session.GrantTasks:     {http.MethodGet, "/tasks"},
	session.GrantActivity:  {http.MethodGet, "/api/activity"},
}

func requestFor(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

// The settable set and the enforced set are one vocabulary. A twelfth grant added
// to either side alone reds here, which is what stops a grant being enforceable
// but unsettable — the shape that let eleven matchers sit behind a field with no
// ingress at all.
func TestEveryGrantNameHasAMatcherAndEveryMatcherHasAName(t *testing.T) {
	enforced := make([]string, 0, len(sessionGrantMatchers))
	for name := range sessionGrantMatchers {
		enforced = append(enforced, name)
	}
	sort.Strings(enforced)

	settable := session.GrantNames()
	sort.Strings(settable)

	if !slices.Equal(enforced, settable) {
		t.Fatalf("the middleware enforces %v and the API accepts %v; one vocabulary, two lists", enforced, settable)
	}
	for name, route := range grantRoutes {
		if _, ok := sessionGrantMatchers[name]; !ok {
			t.Errorf("%s is covered by this test table and is not a grant; the table is describing something that no longer exists", name)
		}
		if !sessionGrantAllows(name, route.method, route.path) {
			t.Errorf("%s does not admit %s %s, so the row below cannot mean what it says", name, route.method, route.path)
		}
	}
	for _, name := range settable {
		if _, ok := grantRoutes[name]; !ok {
			t.Errorf("%s has no route in this table, so it is settable, enforced, and untested", name)
		}
	}
}

// Each grant admits its own group and is refused on every other group's routes.
// Deleting any single matcher arm reds exactly its own row.
func TestEachGrantAdmitsItsOwnGroupAndRefusesTheOthers(t *testing.T) {
	for _, grant := range session.GrantNames() {
		t.Run(grant, func(t *testing.T) {
			sess := &session.Session{Grants: []string{grant}}
			own := grantRoutes[grant]

			if !sessionRequestAllowed(requestFor(own.method, own.path), sess) {
				t.Errorf("a session granted %q was refused %s %s, inside its own group", grant, own.method, own.path)
			}
			for other, route := range grantRoutes {
				if other == grant {
					continue
				}
				if sessionRequestAllowed(requestFor(route.method, route.path), sess) {
					t.Errorf("a session granted only %q reached %s %s, which belongs to %q", grant, route.method, route.path, other)
				}
			}
		})
	}
}

// The two states that admit everything, kept explicit: no grants is "not scoped"
// and "*" is the same thing said out loud.
func TestAnUngrantedSessionIsNotNarrowed(t *testing.T) {
	for name, sess := range map[string]*session.Session{
		"no grants":     {},
		"the wildcard":  {Grants: []string{session.GrantAll}},
		"wildcard plus": {Grants: []string{session.GrantBrowse, session.GrantAll}},
	} {
		for _, route := range grantRoutes {
			if !sessionRequestAllowed(requestFor(route.method, route.path), sess) {
				t.Errorf("%s: %s %s was refused; an unscoped session is narrowed by nothing", name, route.method, route.path)
			}
		}
	}
}

// A grant narrows and never widens: the admin denylist answers before any grant is
// consulted, so no grant can hand a session an admin verb.
func TestNoGrantReachesAnAdminRoute(t *testing.T) {
	admin := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/config"},
		{http.MethodPost, "/shutdown"},
		{http.MethodGet, "/sessions"},
		{http.MethodGet, "/instances"},
		{http.MethodPost, "/cache/clear"},
	}
	all := append(session.GrantNames(), session.GrantAll)
	for _, route := range admin {
		for _, grant := range all {
			sess := &session.Session{Grants: []string{grant}}
			if sessionRequestAllowed(requestFor(route.method, route.path), sess) {
				t.Errorf("grant %q reached the admin route %s %s", grant, route.method, route.path)
			}
		}
	}
}

// The 403 presented two causes and could carry neither. They have different
// remedies — a different credential, or a session with the grant — so the refusal
// has to say which one fired.
func TestTheScopeRefusalNamesWhichCauseFired(t *testing.T) {
	scoped := &session.Session{Grants: []string{session.GrantBrowse}}

	outside, refused := sessionRequestRefusal(requestFor(http.MethodGet, "/clipboard/read"), scoped)
	if !refused {
		t.Fatal("a browse-only session reached the clipboard")
	}
	for _, want := range []string{"browse", "/clipboard/read", "clipboard"} {
		if !strings.Contains(outside.hint, want) {
			t.Errorf("the scope refusal %q does not carry %q — it must name what is held and what would cover the route", outside.hint, want)
		}
	}
	if !strings.Contains(outside.remedy.String(), "--grant") {
		t.Errorf("the scope refusal prescribes %q, not a session carrying the grant", outside.remedy)
	}

	admin, refused := sessionRequestRefusal(requestFor(http.MethodPut, "/api/config"), scoped)
	if !refused {
		t.Fatal("a session reached PUT /api/config")
	}
	if !strings.Contains(admin.hint, "PINCHTAB_SESSION") || !strings.Contains(admin.hint, "server token") {
		t.Errorf("the admin refusal %q does not say which credential arrived or which one is needed", admin.hint)
	}
	if !admin.remedy.Empty() {
		t.Errorf("the admin refusal prescribes %q; the fix is a different credential on whatever verb the caller wanted, which is not one command", admin.remedy)
	}
}

// Grants narrow; server-level gates stay absolute. A grant that appeared to
// re-enable a boot-time capability would be the worst possible outcome of making
// grants reachable, so the composition is asserted rather than reasoned about: the
// middleware admits the route on the grant, and the handler still refuses it.
func TestAGrantDoesNotReopenADisabledServerCapability(t *testing.T) {
	store := session.NewStore(session.Config{Enabled: true, IdleTimeout: 30 * time.Minute, MaxLifetime: 24 * time.Hour})
	sessionID, token, _ := store.Create("test-agent", "", "")
	if !store.SetGrants(sessionID, []string{session.GrantEvaluate}) {
		t.Fatal("the session vanished before it could be scoped")
	}

	h := New(&mockBridge{}, &config.RuntimeConfig{Token: "server-token", AllowEvaluate: false}, nil, nil, nil)
	handler := AuthMiddlewareWithSessions(config.NewLive(h.Config), nil, store, http.HandlerFunc(h.HandleEvaluate))

	req := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(`{"expression":"1+1"}`))
	req.Header.Set("Authorization", "Session "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatal("the evaluate grant reached a handler whose server capability is off")
	}
	if strings.Contains(rr.Body.String(), "session_scope_forbidden") {
		t.Fatalf("the request was refused by SCOPE, not by the server gate, so this test never reached the composition it is about: %s", rr.Body.String())
	}

	// The control: with the capability on, the same grant does reach the handler,
	// so the refusal above is the gate and not the grant.
	h.Config.AllowEvaluate = true
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(`{"expression":"1+1"}`))
	req.Header.Set("Authorization", "Session "+token)
	handler.ServeHTTP(rr, req)
	if strings.Contains(rr.Body.String(), "capability") && rr.Code == http.StatusForbidden {
		t.Errorf("with the capability on the request was still refused by the gate: %s", rr.Body.String())
	}
}
