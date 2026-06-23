package handlers

import (
	"net/http"
	"sort"
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
)

// recordingMux captures registered patterns so a test can assert the live route
// set matches the shared catalog without standing up a real server.
type recordingMux struct{ patterns []string }

func (r *recordingMux) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
}

// expectedRoutes derives the route set the bridge must register from the catalog
// plus the documented special cases: this is the single source of truth the
// registration is verified against.
func expectedRoutes() map[string]bool {
	want := map[string]bool{}
	for _, ep := range routes.Core() {
		if !tabOnlyRoutes[ep.Route()] {
			want[ep.Route()] = true
		}
		if ep.TabScoped {
			want[ep.TabRoute()] = true
		}
	}
	for _, p := range specialCaseRoutes {
		want[p] = true
	}
	return want
}

func TestRegisteredRoutesMatchCatalog(t *testing.T) {
	h := &Handlers{}
	rec := &recordingMux{}
	h.registerBridgeRoutes(rec)
	h.registerSpecialRoutes(rec, func() {})

	got := map[string]bool{}
	for _, p := range rec.patterns {
		if got[p] {
			t.Errorf("route %q registered more than once", p)
		}
		got[p] = true
	}

	want := expectedRoutes()

	for p := range got {
		if !want[p] {
			t.Errorf("registered route %q is not in the catalog or special-case list", p)
		}
	}
	for p := range want {
		if !got[p] {
			t.Errorf("expected route %q was not registered", p)
		}
	}
}

// TestTabOnlyRoutesHaveNoRootForm guards the handoff/resume behavior: they must
// be registered only in their /tabs/{id}/... form, never at the root.
func TestTabOnlyRoutesHaveNoRootForm(t *testing.T) {
	h := &Handlers{}
	rec := &recordingMux{}
	h.registerBridgeRoutes(rec)

	registered := map[string]bool{}
	for _, p := range rec.patterns {
		registered[p] = true
	}
	for route := range tabOnlyRoutes {
		if registered[route] {
			t.Errorf("tab-only route %q must not be registered at the root", route)
		}
	}
}

// TestShutdownRouteIsConditional confirms POST /shutdown registers only when a
// shutdown function is supplied.
func TestShutdownRouteIsConditional(t *testing.T) {
	h := &Handlers{}

	without := &recordingMux{}
	h.registerSpecialRoutes(without, nil)
	for _, p := range without.patterns {
		if p == "POST /shutdown" {
			t.Fatal("POST /shutdown registered with nil shutdown func")
		}
	}

	with := &recordingMux{}
	h.registerSpecialRoutes(with, func() {})
	found := false
	for _, p := range with.patterns {
		if p == "POST /shutdown" {
			found = true
		}
	}
	if !found {
		t.Fatal("POST /shutdown not registered when shutdown func supplied")
	}
}

func TestRegisteredRouteCount(t *testing.T) {
	h := &Handlers{}
	rec := &recordingMux{}
	h.registerBridgeRoutes(rec)
	h.registerSpecialRoutes(rec, func() {})

	if len(rec.patterns) != len(expectedRoutes()) {
		sort.Strings(rec.patterns)
		t.Fatalf("registered %d routes, expected %d", len(rec.patterns), len(expectedRoutes()))
	}
}
