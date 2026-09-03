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
	cfg := o.cfg()
	hasTargets := cfg != nil && len(cfg.Targets) > 0
	if requested == "" {
		var resolved *config.ResolvedBrowserTarget
		resolved, err = config.ResolveDefaultBrowserTarget(cfg)
		if err != nil {
			return "", "", &UnknownBrowserError{Target: requested, Err: err}
		}
		if resolved == nil || resolved.Legacy {
			return "", "", nil
		}
		return resolved.Name, resolved.Provider, nil
	}

	if hasTargets {
		resolved, explicitErr := config.ResolveExplicitBrowserTarget(cfg, requested)
		if explicitErr == nil && resolved != nil && !resolved.Legacy {
			return resolved.Name, resolved.Provider, nil
		}
	}

	if hasTargets {
		target, matches := config.MatchBrowserToTarget(cfg, requested)
		switch {
		case target != "":
			resolved, resolveErr := config.ResolveExplicitBrowserTarget(cfg, target)
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
	if cfg != nil {
		available = cfg.BrowsersAvailable
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
	cfg := o.cfg()
	var fallbacks []string
	if len(fallbackTargets) > 0 {
		fallbacks = fallbackTargets
	} else if strings.TrimSpace(requestedTarget) == "" && resolvedTarget != "" && cfg != nil {
		fallbacks = cfg.FallbackOrder
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
		if cfg == nil || cfg.Targets[fb].Provider == "" {
			var available []string
			if cfg != nil {
				available = cfg.BrowsersAvailable
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
