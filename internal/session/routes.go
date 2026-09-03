package session

import "slices"

// routePatterns is the ONE definition of the agent-session family's mux patterns. Three
// call sites range over it: the live registration in the dashboard package, and the two
// unavailable-mode registrations in the server package. Keeping it here — beside the store
// rather than beside any one registrar — is what makes it impossible for a route to be
// mounted in server mode and answer a bare mux 404 in bridge mode, which is the defect
// this list exists to prevent regenerating one route at a time.
var routePatterns = []string{
	"POST /sessions",
	"GET /sessions",
	"GET /sessions/me",
	"GET /sessions/{id}",
	"POST /sessions/{id}/revoke",
}

// RoutePatterns returns the family's mux patterns. The copy is deliberate: a caller that
// mutated the slice would silently unmount a route in one mode only.
func RoutePatterns() []string {
	return slices.Clone(routePatterns)
}

// The disabled state's vocabulary lives here, beside the route list, because two
// registrars answer for it: the server package when the store booted disabled and
// never mounted the family, and the dashboard's SessionAPI when a save switched
// it off in a process that did mount it.
const (
	CodeDisabled = "sessions_disabled"
	MsgDisabled  = "agent sessions are not enabled on this server"
	HintDisabled = "set sessions.agent.enabled = true and restart the server; the route family is mounted at startup, so enabling it cannot take effect in the running process."
)
