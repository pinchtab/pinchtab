package dashboard

import (
	"encoding/json"
	"path/filepath"

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
	var changed []string
	for _, targets := range []config.BrowserTargetsConfig{current, next} {
		for name := range targets {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			if !sameConfigSection(current[name].Proxy, next[name].Proxy) {
				changed = append(changed, name)
			}
		}
	}
	return changed
}

type bootSnapshottedSetting struct {
	label   string
	changed func(boot, next config.FileConfig) bool
}

var bootSnapshottedSettings = []bootSnapshottedSetting{
	{"Security policy", func(b, n config.FileConfig) bool { return !sameConfigSection(b.Security, n.Security) }},
	{"Activity recording", func(b, n config.FileConfig) bool {
		return !sameConfigSection(b.Observability.Activity.Enabled, n.Observability.Activity.Enabled) ||
			!sameConfigSection(b.Observability.Activity.RetentionDays, n.Observability.Activity.RetentionDays) ||
			!sameConfigSection(b.Observability.Activity.Events, n.Observability.Activity.Events)
	}},
	{"Log level", func(b, n config.FileConfig) bool { return b.Server.LogLevel != n.Server.LogLevel }},
	{"Network recording", func(b, n config.FileConfig) bool {
		return !sameConfigSection(b.Server.NetworkBufferSize, n.Server.NetworkBufferSize) ||
			!sameConfigSection(b.Server.RetainNetworkBodies, n.Server.RetainNetworkBodies) ||
			!sameConfigSection(b.Server.RetainNetworkBodyMaxBytes, n.Server.RetainNetworkBodyMaxBytes)
	}},
	{"Server address", func(b, n config.FileConfig) bool {
		return b.Server.Port != n.Server.Port || b.Server.Bind != n.Server.Bind
	}},
	{"Server state directory (server.stateDir)", func(b, n config.FileConfig) bool {
		return b.Server.StateDir != n.Server.StateDir
	}},
	{"Profiles directory", func(b, n config.FileConfig) bool {
		return effectiveProfilesDir(b) != effectiveProfilesDir(n)
	}},
	{"Profiles configuration", func(b, n config.FileConfig) bool {
		return b.Profiles.DefaultProfile != n.Profiles.DefaultProfile ||
			!sameConfigSection(b.Profiles.QuarantineKeep, n.Profiles.QuarantineKeep)
	}},
	{"Routing strategy", func(b, n config.FileConfig) bool {
		return b.MultiInstance.Strategy != n.MultiInstance.Strategy
	}},
	{"Stealth level", func(b, n config.FileConfig) bool {
		return b.InstanceDefaults.StealthLevel != n.InstanceDefaults.StealthLevel
	}},
	{"Instance defaults", func(b, n config.FileConfig) bool {
		return !sameConfigSection(withoutStealthLevel(b.InstanceDefaults), withoutStealthLevel(n.InstanceDefaults))
	}},
	{"Browser configuration", func(b, n config.FileConfig) bool {
		return !sameConfigSection(b.Browser, n.Browser) ||
			b.Browsers.Default != n.Browsers.Default ||
			!sameConfigSection(b.Browsers.Available, n.Browsers.Available)
	}},
	{"Scheduler configuration", func(b, n config.FileConfig) bool { return !sameConfigSection(b.Scheduler, n.Scheduler) }},
	{"Auto-solver configuration", func(b, n config.FileConfig) bool { return !sameConfigSection(b.AutoSolver, n.AutoSolver) }},
	{"Agent sessions", func(b, n config.FileConfig) bool {
		return !b.Sessions.AgentEnabled() && n.Sessions.AgentEnabled()
	}},
	{"Restart policy", func(b, n config.FileConfig) bool {
		return !sameConfigSection(b.MultiInstance.Restart, n.MultiInstance.Restart)
	}},
}

func (c *ConfigAPI) restartReasonsFor(next config.FileConfig) []string {
	reasons := make([]string, 0, len(bootSnapshottedSettings))
	for _, setting := range bootSnapshottedSettings {
		if setting.changed(c.boot, next) {
			reasons = append(reasons, setting.label)
		}
	}
	return reasons
}

func withoutStealthLevel(defaults config.InstanceDefaultsConfig) config.InstanceDefaultsConfig {
	defaults.StealthLevel = ""
	return defaults
}

func effectiveProfilesDir(fc config.FileConfig) string {
	if fc.Profiles.BaseDir != "" {
		return fc.Profiles.BaseDir
	}
	return filepath.Join(fc.Server.StateDir, "profiles")
}
