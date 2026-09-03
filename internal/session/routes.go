package session

import (
	"slices"

	"github.com/pinchtab/pinchtab/internal/remedy"
)

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
//
// The code and the message are shared because both states genuinely are disabled.
// The HINT is not, and must not be: whether a restart is owed is exactly what
// separates them. A boot-disabled process never mounted the family, so enabling it
// cannot reach the running process; a process disabled by a save already mounted
// the family, so switching it back on applies immediately. One hint serving both
// tells an operator to restart a server that does not need it.
const (
	CodeDisabled = "sessions_disabled"
	MsgDisabled  = "agent sessions are not enabled on this server"

	HintDisabledAtBoot = "set sessions.agent.enabled = true and restart the server; the route family is mounted at startup, so enabling it cannot take effect in the running process."
	HintDisabledBySave = "agent sessions were switched off by a config save; set sessions.agent.enabled = true to switch them back on — this server already mounted the route family, so it applies without a restart."
)

// enableAgentSessions is the remedy the save-disabled state has and the boot-disabled
// state does not: one command, applying live, with no restart to pair it with.
var enableAgentSessions = remedy.Declare("pinchtab config set sessions.agent.enabled true")

// RemedyEnable is the save-disabled state's remedy line.
func RemedyEnable() remedy.Remedy { return enableAgentSessions.Remedy() }
