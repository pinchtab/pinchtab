package handlers

import "github.com/pinchtab/pinchtab/internal/config"

// resolveEffectiveConfig returns a RuntimeConfig with target-specific overrides
// (binary, proxy, Cloak, extraFlags) merged in. The browser parameter is a
// provider name (e.g. "cloak"); it is resolved internally to the matching
// target. When the browser name is empty, targets are not configured, or
// resolution fails, the global h.Config is returned unchanged so the caller
// always gets a usable config.
func (h *Handlers) resolveEffectiveConfig(browser string) (*config.RuntimeConfig, error) {
	if browser == "" || h.Config == nil || len(h.Config.Targets) == 0 {
		return h.Config, nil
	}
	matches := config.TargetsForBrowser(h.Config, browser)
	switch len(matches) {
	case 0:
		return h.Config, nil
	case 1:
		resolved, err := config.ResolveExplicitBrowserTarget(h.Config, matches[0])
		if err != nil {
			return h.Config, nil
		}
		return resolved.Config, nil
	default:
		dt := config.ResolveDefaultTarget(h.Config)
		if dt != "" {
			for _, m := range matches {
				if m == dt {
					resolved, err := config.ResolveExplicitBrowserTarget(h.Config, dt)
					if err != nil {
						return h.Config, nil
					}
					return resolved.Config, nil
				}
			}
		}
		return nil, &config.AmbiguousBrowserError{Browser: browser, Targets: matches}
	}
}
