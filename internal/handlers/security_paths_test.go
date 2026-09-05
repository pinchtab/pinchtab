package handlers

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
)

func TestSecurityStatesListExactlyTheCataloguePathsPerCapability(t *testing.T) {
	states := (&Handlers{}).endpointSecurityStates()
	caps := routes.Capabilities()
	if len(caps) < 5 {
		t.Fatalf("only %d capabilities; this census would prove little", len(caps))
	}
	for _, cap := range caps {
		state, ok := states[string(cap)]
		if !ok {
			t.Errorf("capability %q has no /security state", cap)
			continue
		}
		listed := map[string]bool{}
		for _, path := range state.Paths {
			listed[path] = true
		}
		for _, ep := range routes.CapabilityEndpoints()[cap] {
			if !listed[ep.Route()] {
				t.Errorf("%s omits catalogue route %s", cap, ep.Route())
			}
			if ep.TabScoped && !listed[ep.TabRoute()] {
				t.Errorf("%s omits catalogue route %s", cap, ep.TabRoute())
			}
		}
		if len(state.Paths) == 0 {
			t.Errorf("%s lists no paths", cap)
		}
	}
	for cap, extras := range instanceScopedCapabilityPaths {
		if _, ok := routes.Meta(cap); !ok {
			t.Errorf("instance-scoped extras recorded for %q, which routes.Meta does not describe", cap)
		}
		if len(extras) == 0 {
			t.Errorf("instance-scoped extras for %q are empty; drop the entry", cap)
		}
	}
}
