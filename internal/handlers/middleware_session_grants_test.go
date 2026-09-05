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
	"github.com/pinchtab/pinchtab/internal/routes"
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

// registeredElsewhere are routes a grant matcher admits that the route catalogue
// does not declare because a different mux registers them, each naming its
// registrar. They are recorded rather than exempted by prefix so a matcher cannot
// grow a new path into an unowned namespace unnoticed.
var registeredElsewhere = map[string]string{
	"GET /api/activity":                    "internal/activity.RegisterHandlers on the front door",
	"GET /api/agents/{id}/events":          "internal/dashboard.RegisterHandlers on the front door",
	"POST /api/agents/{id}/events":         "internal/dashboard.RegisterHandlers on the front door",
	"GET /sessions/me":                     "internal/session's SessionAPI on the front door",
	"GET /health":                          "the front door and the bridge each answer their own",
	"GET /openapi.json":                    "served by Handlers.ServeOpenAPI on both front doors",
	"GET /help":                            "the /openapi.json alias",
	"GET /console":                         "bridge diagnostics, registered by registerSpecialRoutes",
	"GET /errors":                          "bridge diagnostics, registered by registerSpecialRoutes",
	"POST /console/clear":                  "bridge diagnostics, registered by registerSpecialRoutes",
	"POST /errors/clear":                   "bridge diagnostics, registered by registerSpecialRoutes",
	"GET /clipboard/read":                  "clipboard family, registered by registerSpecialRoutes",
	"GET /clipboard/paste":                 "clipboard family, registered by registerSpecialRoutes",
	"POST /clipboard/write":                "clipboard family, registered by registerSpecialRoutes",
	"POST /clipboard/copy":                 "clipboard family, registered by registerSpecialRoutes",
	"GET /network/stream":                  "streaming variant, registered by registerSpecialRoutes",
	"GET /network/export":                  "export variant, registered by registerSpecialRoutes",
	"GET /network/export/stream":           "export streaming variant, registered by registerSpecialRoutes",
	"GET /tabs/{id}/network/stream":        "tab-scoped streaming variant",
	"GET /tabs/{id}/network/export":        "tab-scoped export variant",
	"GET /tabs/{id}/network/export/stream": "tab-scoped export streaming variant",
	"POST /lock":                           "tab lock family, registered by registerSpecialRoutes",
	"POST /unlock":                         "tab lock family, registered by registerSpecialRoutes",
	"POST /tabs/{id}/lock":                 "tab lock family, registered by registerSpecialRoutes",
	"POST /tabs/{id}/unlock":               "tab lock family, registered by registerSpecialRoutes",
	"GET /tabs":                            "tab listing, registered by registerSpecialRoutes",
	"POST /tab":                            "tab open/close, registered by registerSpecialRoutes",
	"GET /navigate":                        "the query-parameter form of POST /navigate",
	"GET /action":                          "the query-parameter form of POST /action",
	"GET /config/autosolver":               "autosolver config read, registered by registerSpecialRoutes",
}

// grantRouteUniverse is every route a matcher could be asked about: the catalogue
// (root and tab-scoped forms), the scheduler family, the bridge's special cases,
// and the recorded elsewhere-registered set.
func grantRouteUniverse() map[string]bool {
	universe := map[string]bool{}
	for _, ep := range routes.Core() {
		universe[ep.Route()] = true
		if ep.TabScoped {
			universe[ep.TabRoute()] = true
		}
	}
	for _, ep := range routes.SchedulerEndpoints() {
		universe[ep.Route()] = true
	}
	for _, p := range specialCaseRoutes {
		universe[p] = true
	}
	for route := range registeredElsewhere {
		universe[route] = true
	}
	return universe
}

// routePatternMatches compares a concrete path with a registered pattern, treating
// a {placeholder} as exactly one segment. String equality cannot answer this: a
// matcher is asked about /tasks/abc, and the route that serves it is /tasks/{id}.
func routePatternMatches(pattern, path string) bool {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	if len(patternParts) != len(pathParts) {
		return false
	}
	for i, part := range patternParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			if pathParts[i] == "" {
				return false
			}
			continue
		}
		if part != pathParts[i] {
			return false
		}
	}
	return true
}

func resolvesToARoute(universe map[string]bool, method, path string) bool {
	for route := range universe {
		routeMethod, routePath, ok := strings.Cut(route, " ")
		if !ok || routeMethod != method {
			continue
		}
		if routePatternMatches(routePath, path) {
			return true
		}
	}
	return false
}

// grantProbePaths instantiates every route in the universe as a concrete path, so
// a matcher written as a prefix test is asked about real paths rather than about
// the patterns it never sees at runtime.
func grantProbePaths(universe map[string]bool) []string {
	seen := map[string]bool{}
	var paths []string
	for route := range universe {
		_, path, ok := strings.Cut(route, " ")
		if !ok {
			continue
		}
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				parts[i] = "probe1"
			}
		}
		concrete := strings.Join(parts, "/")
		if !seen[concrete] {
			seen[concrete] = true
			paths = append(paths, concrete)
		}
	}
	sort.Strings(paths)
	return paths
}

// A matcher must not outlive the routes it guards: every path a grant admits has
// to resolve to a route the server actually serves for that method. A grant that
// admits a path nothing serves promises a capability that does not exist, and a
// matcher left behind by a deleted route is invisible until someone tries it.
func TestEveryPathAGrantAdmitsResolvesToARegisteredRoute(t *testing.T) {
	universe := grantRouteUniverse()
	if len(universe) < 100 {
		t.Fatalf("route universe holds %d routes; the census stopped seeing the catalogue", len(universe))
	}
	paths := grantProbePaths(universe)

	admitted := 0
	var dangling []string
	for _, grant := range session.GrantNames() {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPut} {
			for _, path := range paths {
				if !sessionGrantAllows(grant, method, path) {
					continue
				}
				admitted++
				if !resolvesToARoute(universe, method, path) {
					dangling = append(dangling, grant+": "+method+" "+path)
				}
			}
		}
	}
	if admitted < 50 {
		t.Fatalf("the matchers admitted only %d probes; the census would pass vacuously", admitted)
	}

	sort.Strings(dangling)
	if len(dangling) > 0 {
		t.Errorf("%d grant matcher entr(ies) admit a route the server does not serve:\n  %s\nthe grant promises a capability that does not exist; drop the matcher entry, fix its method, or catalogue the route",
			len(dangling), strings.Join(dangling, "\n  "))
	}
}

// Every session must be able to ask what it is. Driven off GrantNames() so a
// twelfth grant is covered the day it is added — a test naming grants literally
// would pass while leaving the new one unable to introspect itself.
func TestEverySessionCanDescribeItselfWhateverItsGrants(t *testing.T) {
	selfDescription := []struct{ method, path string }{
		{http.MethodGet, "/sessions/me"},
		{http.MethodGet, "/health"},
		{http.MethodGet, "/openapi.json"},
		{http.MethodGet, "/help"},
	}

	for _, grant := range session.GrantNames() {
		if grant == session.GrantAll {
			continue
		}
		t.Run(grant, func(t *testing.T) {
			sess := &session.Session{Grants: []string{grant}}
			for _, route := range selfDescription {
				refusal, refused := sessionRequestRefusal(requestFor(route.method, route.path), sess)
				if refused {
					t.Errorf("%s %s is refused under the %q grant (%s); a session that cannot ask what it is has to be told out of band",
						route.method, route.path, grant, refusal.hint)
				}
			}
		})
	}
}
