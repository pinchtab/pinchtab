package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/navguard"
)

// HandleNavigate navigates a tab to a URL or creates a new tab.
//
// @Endpoint POST /navigate
// @Description Navigate to a URL in an existing tab or create a new tab and navigate
//
// @Param tabId string body Tab ID to navigate in (optional - creates new if omitted)
// @Param url string body URL to navigate to (required)
// @Param newTab bool body Force create new tab (optional, default: false)
// @Param waitTitle float64 body Wait for title change (ms) (optional, default: 0)
// @Param timeout float64 body Timeout for navigation (ms) (optional, default: 30000)
//
// @Response 200 application/json Returns {tabId, url, title}
// @Response 400 application/json Invalid URL or parameters
// @Response 500 application/json Chrome error
//
// @Example curl navigate new:
//
//	curl -X POST http://localhost:9867/navigate \
//	  -H "Content-Type: application/json" \
//	  -d '{"url":"https://pinchtab.com"}'
//
// @Example curl navigate existing:
//
//	curl -X POST http://localhost:9867/navigate \
//	  -H "Content-Type: application/json" \
//	  -d '{"tabId":"abc123","url":"https://google.com"}'
//
// @Example cli:
//
//	pinchtab nav https://pinchtab.com
func (h *Handlers) HandleNavigate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeNavigateRequest(w, r)
	if !ok {
		return
	}
	h.navigateToURL(w, r, req)
}

// navigateRequest is the decoded input for the navigate pipeline, shared by
// POST /navigate (GET or JSON body) and the URL form of POST /tab {"action":"new"}.
type navigateRequest struct {
	TabID          string  `json:"tabId"`
	URL            string  `json:"url"`
	NewTab         bool    `json:"newTab"`
	WaitTitle      float64 `json:"waitTitle"`
	Timeout        float64 `json:"timeout"`
	BlockImages    *bool   `json:"blockImages"`
	BlockMedia     *bool   `json:"blockMedia"`
	BlockAds       *bool   `json:"blockAds"`
	WaitFor        string  `json:"waitFor"`
	WaitSelector   string  `json:"waitSelector"`
	DismissBanners bool    `json:"dismissBanners"`
	Browser        string  `json:"browser,omitempty"`
}

// decodeNavigateRequest reads the navigate input from the query string (GET) or
// JSON body (POST). On a malformed request it writes the 400 and returns ok=false.
func decodeNavigateRequest(w http.ResponseWriter, r *http.Request) (navigateRequest, bool) {
	var req navigateRequest
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		d := newQueryDecoder(q)
		req.URL = q.Get("url")
		req.TabID = q.Get("tabId")
		d.Bool("newTab", &req.NewTab)
		req.WaitFor = q.Get("waitFor")
		req.WaitSelector = q.Get("waitSelector")
		d.Bool("dismissBanners", &req.DismissBanners)
		req.Browser = q.Get("browser")
		d.Float("waitTitle", &req.WaitTitle)
		d.Float("timeout", &req.Timeout)
		if err := d.Err(); err != nil {
			httpx.Error(w, 400, err)
			return navigateRequest{}, false
		}
		return req, true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return navigateRequest{}, false
	}
	return req, true
}

// navTargets carries the SSRF-validated navigate target and the trusted-proxy
// CIDR set produced during validation.
type navTargets struct {
	target       *validatedNavigateTarget
	trustedCIDRs []*net.IPNet
}

// navigateToURL runs the full navigate pipeline on an already-decoded request:
// browser resolution, IDPI/target validation, route bookkeeping, current-tab
// scoping, the optional static-first phase, and execute-respond. It is the shared
// core behind POST /navigate and the URL form of POST /tab {"action":"new"}.
func (h *Handlers) navigateToURL(w http.ResponseWriter, r *http.Request, req navigateRequest) {
	tabID := strings.TrimSpace(req.TabID)

	routing, ok := h.resolveNavigateBrowser(w, r, tabID, strings.TrimSpace(req.Browser))
	if !ok {
		return
	}

	targets, ok := h.validateNavigateTargets(w, r, req.TabID, req.URL, routing.EffectiveCfg)
	if !ok {
		return
	}

	navRoute := h.recordNavigateRoute(r, routing)

	scopedCurrentForNavigate, ok := h.applyNavigateTabScope(w, r, &req)
	if !ok {
		return
	}

	sf := h.tryStaticFirstNavigate(w, r, req, routing.EffectiveCfg, navRoute)
	if sf.handled {
		return
	}

	if !h.ensureBrowserOrRespond(w, routing.EffectiveCfg) {
		return
	}

	if sf.escRoute != nil {
		// Merge the static attempt with the Chrome escalation, mirroring the
		// adapter's internal-escalation metadata shape.
		sf.escRoute.Escalated = true
		sf.escRoute.Attempts = append(sf.escRoute.Attempts,
			browserops.RouteAttempt{Browser: "chrome", Accepted: true, Reason: "escalated"})
		navRoute = sf.escRoute
	}

	h.executeNavigate(w, r, req, routing.EffectiveCfg, navRoute, targets, scopedCurrentForNavigate, sf.skipStatic)
}

// resolveNavigateBrowser runs the shared browser-resolution prelude plus the
// navigate-specific ownership-conflict guard: an explicit request/session browser
// must not disagree with the instance that owns the target tab. The compared
// IntentBrowser is the pre-downgrade resolution, not the DecisionSkip fallback
// (ghost-chrome reads run on chrome).
func (h *Handlers) resolveNavigateBrowser(w http.ResponseWriter, r *http.Request, tabID, requestBrowser string) (browserRouting, bool) {
	routing, ok := h.resolveBrowserForRequest(w, r, tabID, requestBrowser, browsers.RequestIntent{
		Shape: browsers.ShapeRenderedRead,
	})
	if !ok {
		return browserRouting{}, false
	}

	// With instance browser feeding resolution above, this only fires when an
	// explicit request/session browser disagrees with the owning instance.
	if routing.IntentBrowser != "" && tabID != "" && h.Orchestrator != nil {
		if inst, ok := h.Orchestrator.FindInstanceByTab(tabID); ok && inst != nil {
			if inst.Browser != "" && config.NormalizeBrowser(inst.Browser) != config.NormalizeBrowser(routing.IntentBrowser) {
				httpx.ErrorCode(w, http.StatusConflict, "browser_conflict",
					fmt.Sprintf("tab %q is owned by instance with browser %q; cannot navigate with browser %q",
						tabID, inst.Browser, routing.IntentBrowser),
					false, map[string]any{
						"tabId":            tabID,
						"instanceBrowser":  inst.Browser,
						"requestedBrowser": routing.IntentBrowser,
					})
				return browserRouting{}, false
			}
		}
	}
	return routing, true
}

// idpiAllowlistHint appends a copy-pasteable remediation to an IDPI domain-block
// error so the user isn't left knowing only the cause. Widening the allowlist
// reduces isolation, so the hint says so and points at the security guide.
func idpiAllowlistHint(url string) string {
	host, ok := navguard.ExtractHost(url)
	if !ok || strings.TrimSpace(host) == "" {
		return ""
	}
	return fmt.Sprintf(". To allow it, run: pinchtab config set security.allowedDomains "+
		"\"$(pinchtab config get security.allowedDomains),%s\" then: pinchtab server restart "+
		"(this widens what automation may reach — see docs/guides/security.md)", host)
}

// idpiScannerHint appends remediation to an IDPI content-scanner block, which
// otherwise states only the cause. Unlike the domain allowlist, a scanner block
// can be a false positive on legitimate pages, so the fix is to relax strict mode
// (still scans and wraps, just warns instead of hard-blocking).
func idpiScannerHint() string {
	return ". To read pages like this, set strict mode off: `pinchtab config set security.idpi.strictMode false` " +
		"then `pinchtab server restart` — content is still scanned and wrapped, just warned instead of blocked (see docs/guides/security.md)"
}

// validateNavigateTargets runs URL validation, the IDPI domain guard, and SSRF
// target resolution, recording the navigate request on both the blocked and
// accepted paths. On success it returns the resolved target and trusted-proxy CIDRs.
func (h *Handlers) validateNavigateTargets(w http.ResponseWriter, r *http.Request, tabID, url string, effectiveCfg *config.RuntimeConfig) (navTargets, bool) {
	allowFile := effectiveCfg != nil && effectiveCfg.AllowFileScheme
	if err := validateNavigateURL(url, allowFile); err != nil {
		httpx.Error(w, 400, err)
		return navTargets{}, false
	}

	domainResult := h.IDPIGuard.CheckDomain(url)
	if domainResult.Blocked {
		h.recordNavigateRequest(r, tabID, url)
		httpx.Error(w, http.StatusForbidden, fmt.Errorf("navigation blocked by IDPI: %s%s", domainResult.Reason, idpiAllowlistHint(url)))
		return navTargets{}, false
	}
	if domainResult.Threat {
		w.Header().Set("X-IDPI-Warning", domainResult.Reason)
	}

	// file:// has no network target, so SSRF/private-IP resolution does not apply.
	// It has already passed the explicit-opt-in scheme gate and the IDPI domain
	// guard above (strict-mode allowlists block it via the empty-host path).
	if allowFile && navguard.IsFileURL(url) {
		h.recordNavigateRequest(r, tabID, url)
		return navTargets{target: &validatedNavigateTarget{AllowInternal: true}, trustedCIDRs: buildNavigateTrustedProxyCIDRs(effectiveCfg)}, true
	}

	trustedResolveCIDRs := parseCIDRs(effectiveCfg.TrustedResolveCIDRs)
	target, err := validateNavigateTarget(url, h.IDPIGuard.DomainAllowed(url), trustedResolveCIDRs)
	if err != nil {
		httpx.Error(w, http.StatusForbidden, err)
		return navTargets{}, false
	}
	trustedCIDRs := buildNavigateTrustedProxyCIDRs(effectiveCfg)
	h.recordNavigateRequest(r, tabID, url)
	return navTargets{target: target, trustedCIDRs: trustedCIDRs}, true
}

// recordNavigateRoute builds the single-browser route metadata for this navigate,
// records it as activity, and returns it for the execute phase (the static-first
// and escalation paths may replace it).
func (h *Handlers) recordNavigateRoute(r *http.Request, routing browserRouting) *browserops.RouteMetadata {
	navRoute := browserops.SingleBrowserRoute(routing.Browser)
	navRoute.Attempts = append(navRoute.Attempts, browserops.RouteAttempt{
		Browser:  routing.Browser,
		Accepted: routing.Decision.Decision == browsers.DecisionHandle,
		Reason:   routing.Decision.Reason,
	})
	if routing.RequestBrowser != "" {
		navRoute.RequestedBrowser = routing.RequestBrowser
	}
	h.recordActivity(r, activity.Update{Route: navRoute})
	return navRoute
}

// applyNavigateTabScope resolves the implicit current-tab pointer when a navigate
// names neither a tab nor newTab, enforcing the strict empty-pointer policy. It
// mutates req.TabID/req.NewTab and reports whether a scoped current tab was
// adopted (so the executor can fall back to a new tab if it has since closed).
func (h *Handlers) applyNavigateTabScope(w http.ResponseWriter, r *http.Request, req *navigateRequest) (scopedCurrent bool, ok bool) {
	explicitTabID := strings.TrimSpace(req.TabID) != ""
	identifiedCaller := !currentTabScopeFromRequest(r).IsGlobal()
	if !explicitTabID && !req.NewTab {
		if scopedTabID, found := h.scopedCurrentTabForRequest(r); found {
			req.TabID = scopedTabID
			scopedCurrent = true
		} else if identifiedCaller && h.EmptyPointerPolicy() == EmptyPointerStrict {
			// Strict empty-pointer policy: identified callers must pin a
			// tab explicitly. Refuse to lazily create one.
			httpx.ErrorCode(w, http.StatusConflict, "no_current_tab",
				"no current tab; explicit tabId required under strict empty-pointer policy",
				false, nil)
			return false, false
		}
	}

	// Default to creating a new tab (API design: /navigate always creates a new
	// tab) unless explicitly reusing an existing tab, or an identified caller has
	// a scoped current tab.
	if req.TabID == "" {
		req.NewTab = true
	}
	return scopedCurrent, true
}

// staticFirstOutcome reports the result of the optional static-first phase.
type staticFirstOutcome struct {
	handled    bool                      // a response was already written
	skipStatic bool                      // the Chrome phase must skip the already-failed static attempt
	escRoute   *browserops.RouteMetadata // partial route from a static escalation, if any
}

// tryStaticFirstNavigate runs the optional ghost-chrome static-first phase for a
// new-tab navigate: a static-capable bridge can serve a fresh navigate without
// launching Chrome (parity with main's lite mode). On a static hit it writes the
// response (handled=true). On escalation it reports skipStatic and any partial
// route so the caller can run the Chrome phase. It is a no-op when the bridge is
// not static-first-capable or this is not a new-tab navigate.
//
// Timeout budget is PER PHASE, not shared: the static attempt gets NavigateTimeout
// (default 30s), and on escalation the Chrome navigate gets a fresh budget of its
// own — worst case a navigate takes up to twice the configured timeout. Sharing
// one budget would let a slow static fetch starve the Chrome attempt that exists
// to rescue it.
func (h *Handlers) tryStaticFirstNavigate(w http.ResponseWriter, r *http.Request, req navigateRequest, effectiveCfg *config.RuntimeConfig, navRoute *browserops.RouteMetadata) staticFirstOutcome {
	sf, ok := h.Bridge.(staticFirstNavigator)
	if !ok || !sf.StaticFirstNavigate() || !req.NewTab {
		return staticFirstOutcome{}
	}

	phase1Timeout := effectiveCfg.NavigateTimeout
	if phase1Timeout <= 0 {
		phase1Timeout = 30 * time.Second
	}
	phase1Ctx, phase1Cancel := context.WithTimeout(r.Context(), phase1Timeout)
	navResult, navErr := h.Bridge.Navigate(phase1Ctx, req.URL, bridge.NavigateParams{
		MaxRedirects: effectiveCfg.MaxRedirects,
		NoEscalate:   true,
	})
	phase1Cancel()
	if navErr == nil && navResult != nil && navResult.TabID != "" {
		if navResult.Route != nil {
			navRoute = navResult.Route
		}
		h.setCurrentTabForRequest(r, navResult.TabID)
		h.recordResolvedURL(r, navResult.URL)
		httpx.JSON(w, 200, map[string]any{"tabId": navResult.TabID, "url": navResult.URL, "title": navResult.Title, "route": navRoute})
		return staticFirstOutcome{handled: true}
	}

	outcome := staticFirstOutcome{skipStatic: true}
	var esc *bridge.StaticEscalateError
	if errors.As(navErr, &esc) && esc.Route != nil {
		outcome.escRoute = esc.Route
	}
	return outcome
}

// navigateBlockPatterns resolves the resource-blocking patterns for a navigate,
// applying any per-request block overrides over the effective-config defaults.
func navigateBlockPatterns(req navigateRequest, effectiveCfg *config.RuntimeConfig) []string {
	blockAds := effectiveCfg.BlockAds
	if req.BlockAds != nil {
		blockAds = *req.BlockAds
	}
	blockMedia := effectiveCfg.BlockMedia
	if req.BlockMedia != nil {
		blockMedia = *req.BlockMedia
	}
	blockImages := effectiveCfg.BlockImages
	if req.BlockImages != nil {
		blockImages = *req.BlockImages
	}

	var blockPatterns []string
	if blockAds {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.AdBlockPatterns)
	}
	if blockMedia {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.MediaBlockPatterns)
	} else if blockImages {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.ImageBlockPatterns)
	}
	return blockPatterns
}

// executeNavigate computes the per-request timeouts and resource-block patterns,
// then dispatches to the new-tab or existing-tab navigate flow.
func (h *Handlers) executeNavigate(w http.ResponseWriter, r *http.Request, req navigateRequest, effectiveCfg *config.RuntimeConfig, navRoute *browserops.RouteMetadata, targets navTargets, scopedCurrentForNavigate, skipStatic bool) {
	titleWait := time.Duration(0)
	if req.WaitTitle > 0 {
		if req.WaitTitle > 30 {
			req.WaitTitle = 30
		}
		titleWait = time.Duration(req.WaitTitle * float64(time.Second))
	}

	navTimeout := effectiveCfg.NavigateTimeout
	if req.Timeout > 0 {
		if req.Timeout > 120 {
			req.Timeout = 120
		}
		navTimeout = time.Duration(req.Timeout * float64(time.Second))
	}

	blockPatterns := navigateBlockPatterns(req, effectiveCfg)

	newTabOpts := func() navigateBrowserOptions {
		return navigateBrowserOptions{
			URL:            req.URL,
			WaitFor:        req.WaitFor,
			WaitSelector:   req.WaitSelector,
			NavTimeout:     navTimeout,
			TitleWait:      titleWait,
			Target:         targets.target,
			TrustedCIDRs:   targets.trustedCIDRs,
			BlockPatterns:  blockPatterns,
			DismissBanners: req.DismissBanners,
			Route:          navRoute,
			MaxRedirects:   effectiveCfg.MaxRedirects,
		}
	}

	if req.NewTab {
		opts := newTabOpts()
		opts.SkipStatic = skipStatic
		h.navigateNewTabBrowser(w, r, opts)
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		if scopedCurrentForNavigate {
			h.navigateNewTabBrowser(w, r, newTabOpts())
			return
		}
		WriteTabContextError(w, err, 404)
		return
	}
	// Navigate signals fresh work on this tab — drop any pending auto-close
	// timer; the next read/action will re-arm.
	h.cancelAutoCloseIfEnabled(resolvedTabID)

	tCtx, tCancel := context.WithTimeout(ctx, navTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)
	h.runNavigate(w, r, navExec{
		tabID:          resolvedTabID,
		ctx:            tCtx,
		cancel:         tCancel,
		url:            req.URL,
		maxRedirects:   effectiveCfg.MaxRedirects,
		route:          navRoute,
		waitFor:        req.WaitFor,
		waitSelector:   req.WaitSelector,
		titleWait:      titleWait,
		dismissBanners: req.DismissBanners,
		target:         targets.target,
		trustedCIDRs:   targets.trustedCIDRs,
		blockPatterns:  blockPatterns,
	})
}

// navExec carries the parameters for a single navigate execution. isNewTab
// selects the blank-tab behaviors (close-on-failure/divergence, recordResolvedTab)
// vs the existing-tab behaviors (DeleteRefCache/clearTabFrameScope, route always
// present in the response, resource-blocking reset on empty patterns).
type navExec struct {
	tabID          string
	ctx            context.Context
	cancel         context.CancelFunc
	url            string
	maxRedirects   int
	skipStatic     bool
	route          *browserops.RouteMetadata
	waitFor        string
	waitSelector   string
	titleWait      time.Duration
	dismissBanners bool
	target         *validatedNavigateTarget
	trustedCIDRs   []*net.IPNet
	blockPatterns  []string
	isNewTab       bool
}

// runNavigate is the shared navigate flow for both the existing-tab and
// blank-tab paths: guard install, resource blocking, Navigate + error
// classification, route extraction, static-first divergence, wait-for, auto-solve
// + banner dismiss, and response assembly. Branch-specific behavior is gated on
// ex.isNewTab so the two callers cannot drift.
func (h *Handlers) runNavigate(w http.ResponseWriter, r *http.Request, ex navExec) {
	navGuard, err := installNavigateRuntimeGuardWithBridge(h.Bridge, ex.ctx, ex.cancel, ex.target, ex.trustedCIDRs)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
		return
	}
	if len(ex.blockPatterns) > 0 {
		_ = bridge.SetResourceBlocking(ex.ctx, ex.blockPatterns)
	} else if !ex.isNewTab {
		_ = bridge.SetResourceBlocking(ex.ctx, nil)
	}

	navResult, navErr := h.Bridge.Navigate(ex.ctx, ex.url, bridge.NavigateParams{
		MaxRedirects: ex.maxRedirects,
		SkipStatic:   ex.skipStatic,
	})
	if navErr != nil {
		if ex.isNewTab {
			// The blank tab never carried the requested URL; keeping it around on
			// failure only burns a MaxTabs slot until eviction.
			_ = h.Bridge.CloseTab(ex.tabID)
		}
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				httpx.Error(w, http.StatusForbidden, blockedErr)
				return
			}
		}
		navigateErrorWithHint(w, classifyNavigateError(navErr), navErr, ex.url)
		return
	}

	route := ex.route
	if navResult != nil && navResult.Route != nil {
		route = navResult.Route
	}

	// Existing tabs may carry stale ref/frame state from a prior page; a fresh
	// blank tab has none.
	if !ex.isNewTab {
		h.Bridge.DeleteRefCache(ex.tabID)
		h.clearTabFrameScope(ex.tabID)
	}

	// The ghost-chrome adapter may serve a navigate from a static tab instead of
	// the tab this handler drove; its result is authoritative. The static
	// document is fully loaded, so waitFor/waitSelector are already satisfied and
	// the remaining post-steps are Chrome-tab CDP ops against a tab that never
	// navigated.
	if navResult != nil && navResult.TabID != "" && navResult.TabID != ex.tabID {
		if ex.isNewTab {
			_ = h.Bridge.CloseTab(ex.tabID)
		}
		h.setCurrentTabForRequest(r, navResult.TabID)
		if ex.isNewTab {
			h.recordResolvedTab(r, navResult.TabID)
		}
		h.recordResolvedURL(r, navResult.URL)
		httpx.JSON(w, 200, navResponse(navResult.TabID, navResult.URL, navResult.Title, route, !ex.isNewTab))
		return
	}

	if err := h.waitForNavigationState(ex.ctx, ex.waitFor, ex.waitSelector); err != nil {
		httpx.ErrorCode(w, 400, "bad_wait_for", err.Error(), false, nil)
		return
	}

	h.maybeAutoSolve(ex.ctx, ex.tabID, autoSolverTriggerNavigate)
	h.dismissBanners(ex.ctx, ex.tabID, ex.dismissBanners)

	navURL, _ := h.Bridge.CurrentURL(ex.ctx)
	title, _ := bridge.WaitForTitle(ex.ctx, ex.titleWait)
	h.setCurrentTabForRequest(r, ex.tabID)
	if ex.isNewTab {
		h.recordResolvedTab(r, ex.tabID)
	}
	h.recordResolvedURL(r, navURL)

	httpx.JSON(w, 200, navResponse(ex.tabID, navURL, title, route, !ex.isNewTab))
}

// classifyNavigateError maps a Navigate error to an HTTP status: 422 for redirect
// overflow, 400 for invalid-URL signals, else 500.
func classifyNavigateError(navErr error) int {
	if errors.Is(navErr, bridge.ErrTooManyRedirects) {
		return 422
	}
	errMsg := navErr.Error()
	if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
		return 400
	}
	return 500
}

// navResponse builds the navigate JSON body. When routeAlways is true the route
// key is always present (existing-tab contract, may be null); otherwise it is
// included only when non-nil (blank-tab contract).
func navResponse(tabID, url, title string, route *browserops.RouteMetadata, routeAlways bool) map[string]any {
	resp := map[string]any{"tabId": tabID, "url": url, "title": title}
	if routeAlways || route != nil {
		resp["route"] = route
	}
	return resp
}

type navigateBrowserOptions struct {
	URL            string
	WaitFor        string
	WaitSelector   string
	NavTimeout     time.Duration
	TitleWait      time.Duration
	Target         *validatedNavigateTarget
	TrustedCIDRs   []*net.IPNet
	BlockPatterns  []string
	DismissBanners bool
	Route          *browserops.RouteMetadata
	MaxRedirects   int
	// SkipStatic: the handler already ran (and failed) the static-first
	// phase; the bridge must go straight to Chrome.
	SkipStatic bool
}

// staticFirstNavigator is probed on the bridge to enable the deferred-launch
// two-phase navigate (NoEscalate, then SkipStatic on escalation).
type staticFirstNavigator interface {
	StaticFirstNavigate() bool
}

func (h *Handlers) navigateNewTabBrowser(w http.ResponseWriter, r *http.Request, opts navigateBrowserOptions) {
	// Create a blank tab first so the requested URL becomes the first
	// real history entry.
	newTabID, newCtx, _, err := h.Bridge.CreateTab("")
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("new tab: %w", err))
		return
	}

	tCtx, tCancel := context.WithTimeout(newCtx, opts.NavTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	h.runNavigate(w, r, navExec{
		tabID:          newTabID,
		ctx:            tCtx,
		cancel:         tCancel,
		url:            opts.URL,
		maxRedirects:   opts.MaxRedirects,
		skipStatic:     opts.SkipStatic,
		route:          opts.Route,
		waitFor:        opts.WaitFor,
		waitSelector:   opts.WaitSelector,
		titleWait:      opts.TitleWait,
		dismissBanners: opts.DismissBanners,
		target:         opts.Target,
		trustedCIDRs:   opts.TrustedCIDRs,
		blockPatterns:  opts.BlockPatterns,
		isNewTab:       true,
	})
}

// @Endpoint POST /tabs/{id}/navigate
func (h *Handlers) HandleTabNavigate(w http.ResponseWriter, r *http.Request) {
	// Path tab ID is canonical for this endpoint and always navigates the
	// existing tab, so force newTab=false on the forwarded body.
	h.withPathTabIDBodyMutate(w, r, func(body map[string]any) {
		body["newTab"] = false
	}, h.HandleNavigate)
}

// binaryFileExtensions are extensions that Chrome cannot render and will abort on
var binaryFileExtensions = []string{".gz", ".zip", ".tar", ".rar", ".7z", ".bz2", ".xz", ".pdf", ".exe", ".bin", ".dmg", ".iso"}

func isNavigateAbortedOnBinary(err error, url string) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "ERR_ABORTED") {
		return false
	}
	lowerURL := strings.ToLower(url)
	for _, ext := range binaryFileExtensions {
		if strings.HasSuffix(lowerURL, ext) || strings.Contains(lowerURL, ext+"?") {
			return true
		}
	}
	return false
}

func navigateErrorWithHint(w http.ResponseWriter, code int, err error, url string) {
	if isNavigateAbortedOnBinary(err, url) {
		httpx.ErrorCode(w, 502, "nav_binary_aborted", fmt.Sprintf("navigate: %s", err.Error()), false, map[string]any{
			"remedy": "download",
			"hint":   fmt.Sprintf("Chrome cannot render binary/compressed files. Use: pinchtab download %q", url),
		})
		return
	}
	httpx.Error(w, code, fmt.Errorf("navigate: %w", err))
}
