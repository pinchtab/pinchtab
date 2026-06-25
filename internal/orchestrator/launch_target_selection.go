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

	if o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		resolved, explicitErr := config.ResolveExplicitBrowserTarget(o.runtimeCfg, requested)
		if explicitErr == nil && resolved != nil && !resolved.Legacy {
			return resolved.Name, resolved.Provider, nil
		}
	}

	if o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		target, matches := config.MatchBrowserToTarget(o.runtimeCfg, requested)
		switch {
		case target != "":
			resolved, resolveErr := config.ResolveExplicitBrowserTarget(o.runtimeCfg, target)
			if resolveErr == nil && resolved != nil {
				return resolved.Name, resolved.Provider, nil
			}
			// A configured target name failing to resolve is unreachable in
			// practice; fall through to the legacy parse below if it ever does.
		case len(matches) == 0:
			return "", "", &UnknownBrowserError{Target: requested, Err: fmt.Errorf("no browser target configured for browser %q", requested)}
		default:
			return "", "", &config.AmbiguousBrowserError{Browser: requested, Targets: matches}
		}
	}

	// Legacy (no-targets) fallthrough: an explicit unknown browser must fail
	// here, not silently launch chrome via NormalizeBrowser coercion later.
	var available []string
	if o.runtimeCfg != nil {
		available = o.runtimeCfg.BrowsersAvailable
	}
	parsed, err := config.ParseBrowser(requested, available)
	if err != nil {
		return "", "", &UnknownBrowserError{Target: requested, Err: err}
	}
	return "", parsed, nil
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
	opts.TargetName = resolvedTarget

	// Fallback policy: request-supplied fallbackTargets are always honored.
	// The config-level fallbackOrder applies only to IMPLICIT launches — an
	// explicit browser/target request must never silently change provider via
	// operator-configured fallback. Fallback entries may be provider names
	// (e.g. "cloak") or target names (e.g. "cloak-1"); each is resolved through
	// the same two-step logic as the primary request, so a provider-name
	// fallback no longer 400s and aborts the chain.
	var fallbacks []string
	if len(fallbackTargets) > 0 {
		fallbacks = fallbackTargets
	} else if strings.TrimSpace(requestedTarget) == "" && resolvedTarget != "" && o.runtimeCfg != nil {
		fallbacks = o.runtimeCfg.FallbackOrder
	}

	if resolvedTarget == "" || len(fallbacks) == 0 {
		return o.LaunchWithOptions(name, port, headless, opts)
	}

	candidates := []string{resolvedTarget}
	for _, fb := range fallbacks {
		fb = strings.TrimSpace(fb)
		if fb == "" {
			continue
		}
		// Reject a fallback that is neither a configured target nor a known
		// provider before resolving: the two-step resolver's NormalizeBrowser
		// default would otherwise silently coerce a typo'd name to chrome.
		if o.runtimeCfg == nil || o.runtimeCfg.Targets[fb].Provider == "" {
			var available []string
			if o.runtimeCfg != nil {
				available = o.runtimeCfg.BrowsersAvailable
			}
			if _, perr := config.ParseBrowser(fb, available); perr != nil {
				return nil, &UnknownBrowserError{Target: fb, Err: perr}
			}
		}
		fbTarget, _, fbErr := o.ResolveRequestedBrowser(fb)
		if fbErr != nil {
			return nil, fbErr
		}
		if fbTarget == "" {
			continue
		}
		candidates = append(candidates, fbTarget)
	}
	return o.LaunchWithFallback(name, port, headless, candidates, opts)
}
