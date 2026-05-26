package handlers

import "github.com/pinchtab/pinchtab/internal/config"

// resolveEffectiveConfig returns a RuntimeConfig with target-specific overrides
// (binary, proxy, Cloak, extraFlags) merged in. When the target name is empty,
// targets are not configured, or resolution fails, the global h.Config is
// returned unchanged so the caller always gets a usable config.
func (h *Handlers) resolveEffectiveConfig(targetName string) *config.RuntimeConfig {
	if targetName == "" || h.Config == nil || len(h.Config.Targets) == 0 {
		return h.Config
	}
	resolved, err := config.ResolveExplicitBrowserTarget(h.Config, targetName)
	if err != nil {
		return h.Config
	}
	return resolved.Config
}
