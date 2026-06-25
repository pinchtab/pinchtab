package handlers

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// rejectBrowserConflictWithRunning writes a 409 and reports true when an
// explicitly requested browser (request param, falling back to session) cannot
// be honored because a different browser process is already running.
// EnsureBrowser ignores the requested config once initialized, so without this
// check an explicit mismatch silently executes on the wrong browser — a
// stealth regression. Implicit resolution (global default, instance-derived,
// capability downgrades) must not be passed here.
func (h *Handlers) rejectBrowserConflictWithRunning(w http.ResponseWriter, requestBrowser, sessionBrowser string) bool {
	explicit := requestBrowser
	if explicit == "" {
		explicit = sessionBrowser
	}
	if explicit == "" || h.Bridge == nil {
		return false
	}
	// Unknown names must fail validation (400) before the conflict check:
	// NormalizeBrowser coerces unknowns to chrome, so comparing them against
	// the running browser would emit a 409 advising a restart with a browser
	// that doesn't exist.
	if _, err := config.ParseBrowser(explicit, h.Config.BrowsersAvailable); err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return true
	}
	running, ok := h.Bridge.RunningBrowser()
	if !ok || running == "" {
		return false
	}
	if config.NormalizeBrowser(explicit) == config.NormalizeBrowser(running) {
		return false
	}
	httpx.ErrorCode(w, http.StatusConflict, "browser_conflict",
		fmt.Sprintf("browser %q requested but %q is already running; restart the server with --browser %s or launch an instance with that browser", explicit, running, explicit),
		false, map[string]any{
			"requestedBrowser": explicit,
			"runningBrowser":   running,
		})
	return true
}

// resolveEffectiveConfig returns a RuntimeConfig with target-specific overrides
// (binary, proxy, Cloak, extraFlags) merged in. The browser parameter is a
// provider name (e.g. "cloak"); it is resolved internally to the matching
// target. An empty browser name, no configured targets, or no target for the
// provider legitimately fall back to the global h.Config — but a target that
// EXISTS and fails to resolve is an error: silently substituting the global
// config would run the request with the wrong binary/proxy/fingerprint.
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
			return nil, fmt.Errorf("resolve browser target %q: %w", matches[0], err)
		}
		return resolved.Config, nil
	default:
		dt := config.ResolveDefaultTarget(h.Config)
		if dt != "" {
			for _, m := range matches {
				if m == dt {
					resolved, err := config.ResolveExplicitBrowserTarget(h.Config, dt)
					if err != nil {
						return nil, fmt.Errorf("resolve default browser target %q: %w", dt, err)
					}
					return resolved.Config, nil
				}
			}
		}
		return nil, &config.AmbiguousBrowserError{Browser: browser, Targets: matches}
	}
}
