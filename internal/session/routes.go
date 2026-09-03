package session

import (
	"fmt"
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
// The GUIDANCE is not, and must not be, because it varies in two independent ways.
// Whether a RESTART is owed separates the two registrars: a boot-disabled process
// never mounted the family, so enabling it cannot reach the running process, while
// a save-disabled one already mounted it and re-enabling applies immediately. WHICH
// SETTING is off separates two ways of being disabled: enabled false and mode off
// both reach this refusal, and a refusal naming the wrong one hands the operator a
// command that leaves them on the identical refusal with nothing new.
const (
	CodeDisabled = "sessions_disabled"
	MsgDisabled  = "agent sessions are not enabled on this server"

	SettingEnabled = "sessions.agent.enabled"
	SettingMode    = "sessions.agent.mode"
)

const (
	hintAtBoot = "set %s and restart the server; the route family is mounted at startup, so enabling it cannot take effect in the running process."
	hintBySave = "agent sessions were switched off by a config save; set %s to switch them back on — this server already mounted the route family, so it applies without a restart."
)

var (
	enableAgentSessions  = remedy.Declare("pinchtab config set " + SettingEnabled + " true")
	preferAgentSessions  = remedy.Declare("pinchtab config set " + SettingMode + " preferred")
	restoreAgentSessions = remedy.Declare("pinchtab config set " + SettingEnabled + " true && pinchtab config set " + SettingMode + " preferred")
)

// DisabledGuidance is the hint and remedy for a disabled refusal. off is what
// Store.DisabledBy answered; mounted says whether this process mounted the family,
// which is what decides whether any single command can reach it.
func DisabledGuidance(off []string, mounted bool) (string, remedy.Remedy) {
	setting, r := SettingEnabled+" = true", enableAgentSessions.Remedy()
	switch {
	case slices.Contains(off, SettingEnabled) && slices.Contains(off, SettingMode):
		setting, r = SettingEnabled+" = true and "+SettingMode+" = preferred", restoreAgentSessions.Remedy()
	case slices.Contains(off, SettingMode):
		setting, r = SettingMode+" = preferred", preferAgentSessions.Remedy()
	}
	if !mounted {
		return fmt.Sprintf(hintAtBoot, setting), remedy.None
	}
	return fmt.Sprintf(hintBySave, setting), r
}
