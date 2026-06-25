package server

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
)

// TestDefaultProxyShorthandsAreCatalogRoutes guards against the default-proxy
// allowlist drifting from the shared route catalog: every forwarded route must
// be a real catalog route (or the management GET /tabs handled separately), so a
// rename/removal/typo can never leave the landing proxy forwarding a dead route.
func TestDefaultProxyShorthandsAreCatalogRoutes(t *testing.T) {
	catalog := map[string]bool{}
	for _, ep := range routes.Core() {
		catalog[ep.Route()] = true
		if ep.TabScoped {
			catalog[ep.TabRoute()] = true
		}
	}
	// GET /tabs is a management route registered outside the catalog (it has a
	// bespoke empty-instances response); allow it explicitly.
	managementAllow := map[string]bool{"GET /tabs": true}

	for _, route := range DefaultProxyShorthands {
		if !catalog[route] && !managementAllow[route] {
			t.Errorf("default-proxy route %q is not a catalog route", route)
		}
	}
	if managementAllow["GET /tabs"] && catalog["GET /tabs"] {
		t.Error("GET /tabs is now a catalog route; drop it from managementAllow")
	}
}
