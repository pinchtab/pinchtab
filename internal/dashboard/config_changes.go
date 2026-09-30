package dashboard

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
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
	effectiveCurrent, effectiveNext := config.EffectiveFileConfig(*current), config.EffectiveFileConfig(*next)
	current, next = &effectiveCurrent, &effectiveNext
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
	union := make(map[string]struct{}, len(current)+len(next))
	for name := range current {
		union[name] = struct{}{}
	}
	for name := range next {
		union[name] = struct{}{}
	}
	names := slices.Sorted(maps.Keys(union))
	changed := names[:0]
	for _, name := range names {
		if !sameConfigSection(current[name].Proxy, next[name].Proxy) {
			changed = append(changed, name)
		}
	}
	return changed
}

func (c *ConfigAPI) restartReasonsFor(next config.FileConfig) []string {
	reasons := make([]string, 0, 8)
	boot, next := config.EffectiveFileConfig(c.boot), config.EffectiveFileConfig(next)

	if !sameConfigSection(boot.Security, next.Security) {
		reasons = append(reasons, "Security policy")
	}
	if boot.Server.Port != next.Server.Port || boot.Server.Bind != next.Server.Bind {
		reasons = append(reasons, "Server address")
	}
	if boot.Server.StateDir != next.Server.StateDir {
		reasons = append(reasons, "Server state directory (server.stateDir)")
	}
	if effectiveProfilesDir(boot) != effectiveProfilesDir(next) {
		reasons = append(reasons, "Profiles directory")
	}
	if boot.MultiInstance.Strategy != next.MultiInstance.Strategy {
		reasons = append(reasons, "Routing strategy")
	}
	if boot.InstanceDefaults.StealthLevel != next.InstanceDefaults.StealthLevel {
		reasons = append(reasons, "Stealth level")
	}
	if !boot.Sessions.AgentEnabled() && next.Sessions.AgentEnabled() {
		reasons = append(reasons, "Agent sessions")
	}
	if !sameConfigSection(boot.MultiInstance.Restart, next.MultiInstance.Restart) {
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
