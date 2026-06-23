package handlers

import (
	"errors"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
)

// browserRouting is the result of the shared browser-selection + capability
// routing prelude.
type browserRouting struct {
	Browser        string // post-CanHandle-downgrade: the browser that launches
	IntentBrowser  string // pre-downgrade resolution (e.g. navigation ownership conflict)
	RequestBrowser string // raw ?browser= value (for route.RequestedBrowser)
	EffectiveCfg   *config.RuntimeConfig
	Decision       browsers.HandleDecision
}

// resolveBrowserForRequest runs the shared browser-selection + capability-routing
// prelude: request/session/instance resolution, running-browser conflict check,
// name validation, CanHandle (downgrading to chrome on skip), and effective-config
// resolution (handling ambiguity). The caller supplies requestBrowser (from the
// query for GET handlers, or the request body for POST handlers). On any client
// error it writes the response and returns ok=false; callers must return
// immediately when ok is false.
func (h *Handlers) resolveBrowserForRequest(w http.ResponseWriter, r *http.Request, tabID, requestBrowser string,
	intent browsers.RequestIntent) (browserRouting, bool) {
	var sessionBrowser string
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		sessionBrowser = sess.Browser
	}
	if h.rejectBrowserConflictWithRunning(w, requestBrowser, sessionBrowser) {
		return browserRouting{}, false
	}
	var instanceBrowser string
	if tabID != "" && h.Orchestrator != nil {
		if inst, ok := h.Orchestrator.FindInstanceByTab(tabID); ok && inst != nil && inst.Browser != "" {
			instanceBrowser = inst.Browser
		}
	}

	resolved := config.ResolveBrowser(requestBrowser, sessionBrowser, instanceBrowser, h.Config.DefaultBrowser, h.Config.BrowsersAvailable)
	if resolved != config.BrowserChrome {
		if _, err := config.ParseBrowser(resolved, h.Config.BrowsersAvailable); err != nil {
			httpx.Error(w, http.StatusBadRequest, err)
			return browserRouting{}, false
		}
	}

	intentBrowser := resolved
	decision, err := checkBrowserCanHandle(resolved, intent)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return browserRouting{}, false
	}
	if decision.Decision == browsers.DecisionSkip {
		resolved = config.BrowserChrome
	}

	effectiveCfg, err := h.resolveEffectiveConfig(resolved)
	if err != nil {
		var ambErr *config.AmbiguousBrowserError
		if errors.As(err, &ambErr) {
			httpx.ErrorCode(w, http.StatusBadRequest, "browser_ambiguous", err.Error(), false, map[string]any{
				"browser": ambErr.Browser,
				"targets": ambErr.Targets,
			})
		} else {
			httpx.Error(w, http.StatusBadRequest, err)
		}
		return browserRouting{}, false
	}

	return browserRouting{
		Browser:        resolved,
		IntentBrowser:  intentBrowser,
		RequestBrowser: requestBrowser,
		EffectiveCfg:   effectiveCfg,
		Decision:       decision,
	}, true
}
