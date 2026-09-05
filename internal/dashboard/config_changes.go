package dashboard

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

func sameConfigSection(a, b any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	if errLeft != nil || errRight != nil {
		return false
	}
	return string(left) == string(right)
}

type sensitiveConfigChangeSet struct {
	requiresElevation bool
	proxyChanged      bool
	names             []string
	proxyScopes       []string
	proxyAudit        []proxyAuditChange
}

type proxyAuditChange struct {
	Scope  string `json:"scope"`
	Server string `json:"server"`
}

func sensitiveConfigChanges(current, next *config.FileConfig) sensitiveConfigChangeSet {
	var out sensitiveConfigChangeSet
	if current == nil || next == nil {
		return out
	}
	if !sameConfigSection(current.Security, next.Security) {
		out.requiresElevation = true
		out.names = append(out.names, "security")
	}
	if !sameConfigSection(current.Browser.Proxy, next.Browser.Proxy) {
		out.requiresElevation = true
		out.proxyChanged = true
		out.names = append(out.names, "browser.proxy")
		out.proxyScopes = append(out.proxyScopes, "browser.proxy")
		out.proxyAudit = append(out.proxyAudit, proxyAuditChange{
			Scope:  "browser.proxy",
			Server: next.Browser.Proxy.Redacted().Server,
		})
	}
	for _, name := range changedTargetProxyNames(current.Browser.Targets, next.Browser.Targets) {
		out.requiresElevation = true
		out.proxyChanged = true
		field := "browser.targets." + name + ".proxy"
		out.names = append(out.names, field)
		out.proxyScopes = append(out.proxyScopes, field)
		out.proxyAudit = append(out.proxyAudit, proxyAuditChange{
			Scope:  field,
			Server: next.Browser.Targets[name].Proxy.Redacted().Server,
		})
	}
	return out
}

func changedTargetProxyNames(current, next config.BrowserTargetsConfig) []string {
	seen := make(map[string]struct{}, len(current)+len(next))
	names := make([]string, 0, len(current)+len(next))
	for name := range current {
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	for name := range next {
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	var changed []string
	for _, name := range names {
		if !sameConfigSection(current[name].Proxy, next[name].Proxy) {
			changed = append(changed, name)
		}
	}
	return changed
}

func (c *ConfigAPI) restartReasonsFor(next config.FileConfig) []string {
	reasons := make([]string, 0, 8)

	// The IDPI guard, allowlist, and sensitive-endpoint policy are snapshotted
	// from the boot config when the server starts and are not rebuilt on a config
	// edit, so any change to the security block only takes effect after a restart.
	// Surfacing it here is what lets `pinchtab security`/`health` warn that the
	// running server is enforcing stale policy instead of silently diverging.
	if !sameConfigSection(c.boot.Security, next.Security) {
		reasons = append(reasons, "Security policy")
	}
	if c.boot.Server.Port != next.Server.Port || c.boot.Server.Bind != next.Server.Bind {
		reasons = append(reasons, "Server address")
	}
	if c.boot.Server.StateDir != next.Server.StateDir {
		reasons = append(reasons, "Server state directory (server.stateDir)")
	}
	if effectiveProfilesDir(c.boot) != effectiveProfilesDir(next) {
		reasons = append(reasons, "Profiles directory")
	}
	if c.boot.MultiInstance.Strategy != next.MultiInstance.Strategy {
		reasons = append(reasons, "Routing strategy")
	}
	if c.boot.InstanceDefaults.StealthLevel != next.InstanceDefaults.StealthLevel {
		reasons = append(reasons, "Stealth level")
	}
	// The sessions.agent block applies live in every direction but one: whether
	// the session API and its route family exist at all is decided at boot, so a
	// process that booted with agent sessions off cannot serve them until it
	// restarts. Turning them off needs no restart — both request-time consumers
	// read the store live — and claiming otherwise about an authentication
	// control would be the same defect one direction over.
	if !c.boot.Sessions.AgentEnabled() && next.Sessions.AgentEnabled() {
		reasons = append(reasons, "Agent sessions")
	}
	if !sameIntPtr(c.boot.MultiInstance.Restart.MaxRestarts, next.MultiInstance.Restart.MaxRestarts) ||
		!sameIntPtr(c.boot.MultiInstance.Restart.InitBackoffSec, next.MultiInstance.Restart.InitBackoffSec) ||
		!sameIntPtr(c.boot.MultiInstance.Restart.MaxBackoffSec, next.MultiInstance.Restart.MaxBackoffSec) ||
		!sameIntPtr(c.boot.MultiInstance.Restart.StableAfterSec, next.MultiInstance.Restart.StableAfterSec) {
		reasons = append(reasons, "Restart policy")
	}

	return reasons
}

func effectiveProfilesDir(fc config.FileConfig) string {
	if baseDir := strings.TrimSpace(fc.Profiles.BaseDir); baseDir != "" {
		return filepath.Clean(baseDir)
	}
	return filepath.Join(strings.TrimSpace(fc.Server.StateDir), "profiles")
}

func sameIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
