package orchestrator

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ResolveRequestedBrowser resolves a public browser name (provider like
// "cloak") or target name (like "cloak-1") against the current runtime
// config and wraps failures for HTTP handlers.
func (o *Orchestrator) ResolveRequestedBrowser(requested string) (targetName, provider string, err error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		var resolved *config.ResolvedBrowserTarget
		resolved, err = config.ResolveDefaultBrowserTarget(o.runtimeCfg)
		if err != nil {
			return "", "", &UnknownBrowserError{Target: requested, Err: err}
		}
		if resolved == nil || resolved.Legacy {
			return "", "", nil
		}
		return resolved.Name, resolved.Provider, nil
	}

	// Try as explicit target name first.
	if o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		resolved, explicitErr := config.ResolveExplicitBrowserTarget(o.runtimeCfg, requested)
		if explicitErr == nil && resolved != nil && !resolved.Legacy {
			return resolved.Name, resolved.Provider, nil
		}
	}

	// Fall back to browser (provider) name resolution.
	if o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		matches := config.TargetsForBrowser(o.runtimeCfg, requested)
		switch len(matches) {
		case 1:
			resolved, resolveErr := config.ResolveExplicitBrowserTarget(o.runtimeCfg, matches[0])
			if resolveErr == nil && resolved != nil {
				return resolved.Name, resolved.Provider, nil
			}
		case 0:
			// No matching target for this browser — treat as unknown.
			return "", "", &UnknownBrowserError{Target: requested, Err: fmt.Errorf("no browser target configured for browser %q", requested)}
		default:
			dt := config.ResolveDefaultTarget(o.runtimeCfg)
			for _, m := range matches {
				if m == dt {
					resolved, resolveErr := config.ResolveExplicitBrowserTarget(o.runtimeCfg, dt)
					if resolveErr == nil && resolved != nil {
						return resolved.Name, resolved.Provider, nil
					}
				}
			}
			return "", "", &config.AmbiguousBrowserError{Browser: requested, Targets: matches}
		}
	}

	return "", "", nil
}

// LaunchWithTargetSelection applies browserTarget/defaultTarget/fallbackOrder
// consistently across orchestration entry points.
func (o *Orchestrator) LaunchWithTargetSelection(
	name, port string,
	headless bool,
	requestedTarget string,
	fallbackTargets []string,
	opts LaunchOptions,
) (*bridge.Instance, error) {
	resolvedTarget, resolvedProvider, err := o.ResolveRequestedBrowser(requestedTarget)
	if err != nil {
		return nil, err
	}

	opts.RequestedProvider = requestedTarget
	opts.Browser = resolvedProvider

	var fallbacks []string
	if len(fallbackTargets) > 0 {
		fallbacks = fallbackTargets
	} else if resolvedTarget != "" && o.runtimeCfg != nil {
		fallbacks = o.runtimeCfg.FallbackOrder
	}

	if resolvedTarget == "" || len(fallbacks) == 0 {
		return o.LaunchWithOptions(name, port, headless, opts)
	}

	candidates := append([]string{resolvedTarget}, fallbacks...)
	return o.LaunchWithFallback(name, port, headless, candidates, opts)
}
