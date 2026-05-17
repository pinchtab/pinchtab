package orchestrator

import (
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ResolveRequestedBrowserTarget resolves a public browserTarget value against
// the current runtime config and wraps failures for HTTP handlers.
func (o *Orchestrator) ResolveRequestedBrowserTarget(requested string) (targetName, provider string, err error) {
	var resolved *config.ResolvedBrowserTarget
	if strings.TrimSpace(requested) == "" {
		resolved, err = config.ResolveDefaultBrowserTarget(o.runtimeCfg)
	} else {
		resolved, err = config.ResolveExplicitBrowserTarget(o.runtimeCfg, requested)
	}
	if err != nil {
		return "", "", &UnknownBrowserTargetError{Target: requested, Err: err}
	}
	if resolved == nil || resolved.Legacy {
		return "", "", nil
	}
	return resolved.Name, resolved.Provider, nil
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
	resolvedTarget, resolvedProvider, err := o.ResolveRequestedBrowserTarget(requestedTarget)
	if err != nil {
		return nil, err
	}

	opts.RequestedBrowserTarget = requestedTarget
	opts.BrowserTarget = resolvedTarget
	opts.BrowserProvider = resolvedProvider

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
