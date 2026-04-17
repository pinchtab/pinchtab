package routes

import (
	"strings"
	"testing"
)

func TestFrameEndpointsAreShorthandAndTabScoped(t *testing.T) {
	found := map[string]bool{
		"GET /frame":  false,
		"POST /frame": false,
	}
	for _, route := range ShorthandRoutes() {
		if _, ok := found[route]; ok {
			found[route] = true
		}
	}
	for route, ok := range found {
		if !ok {
			t.Fatalf("ShorthandRoutes() missing %s", route)
		}
	}

	wantTabRoutes := map[string]bool{
		"GET /tabs/{id}/frame":  false,
		"POST /tabs/{id}/frame": false,
	}
	for _, route := range TabScopedRoutes() {
		if _, ok := wantTabRoutes[route]; ok {
			wantTabRoutes[route] = true
		}
	}
	for route, ok := range wantTabRoutes {
		if !ok {
			t.Fatalf("TabScopedRoutes() missing %s", route)
		}
	}
}

func TestNoDuplicateRoutes(t *testing.T) {
	seen := make(map[string]bool)
	for _, ep := range Core() {
		key := ep.Route()
		if seen[key] {
			t.Errorf("duplicate route: %s", key)
		}
		seen[key] = true
	}
}

func TestShorthandRoutesExcludeCapabilityGated(t *testing.T) {
	for _, r := range ShorthandRoutes() {
		for _, ep := range Core() {
			if ep.Route() == r && ep.Capability != CapNone {
				t.Errorf("ShorthandRoutes() includes capability-gated route: %s", r)
			}
		}
	}
}

func TestTabScopedRoutesFormat(t *testing.T) {
	for _, r := range TabScopedRoutes() {
		if !strings.Contains(r, "/tabs/{id}/") {
			t.Errorf("TabScopedRoutes() returned non-tab-scoped route: %s", r)
		}
	}
}

func TestTabRouteOnNonTabScopedPanics(t *testing.T) {
	ep := Endpoint{Method: "POST", Path: "/tab", TabScoped: false}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected TabRoute() to panic on non-tab-scoped endpoint")
		}
	}()
	ep.TabRoute()
}

func TestCapabilityEndpointsGrouping(t *testing.T) {
	caps := CapabilityEndpoints()
	if len(caps) == 0 {
		t.Fatal("expected some capability-gated endpoints")
	}
	for cap, eps := range caps {
		for _, ep := range eps {
			if ep.Capability != cap {
				t.Errorf("endpoint %s grouped under %s but has capability %s", ep.Route(), cap, ep.Capability)
			}
		}
	}
}

func TestAllEndpointsHaveMethodAndPath(t *testing.T) {
	for _, ep := range Core() {
		if ep.Method == "" {
			t.Errorf("endpoint with empty method: %+v", ep)
		}
		if ep.Path == "" || ep.Path[0] != '/' {
			t.Errorf("endpoint with invalid path: %+v", ep)
		}
		if ep.Summary == "" {
			t.Errorf("endpoint with empty summary: %s", ep.Route())
		}
	}
}
