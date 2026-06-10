package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
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
	var req struct {
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

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.URL = q.Get("url")
		req.TabID = q.Get("tabId")
		req.NewTab = strings.EqualFold(q.Get("newTab"), "true") || q.Get("newTab") == "1"
		req.WaitFor = q.Get("waitFor")
		req.WaitSelector = q.Get("waitSelector")
		req.DismissBanners = strings.EqualFold(q.Get("dismissBanners"), "true") || q.Get("dismissBanners") == "1"
		req.Browser = q.Get("browser")
		if v := q.Get("waitTitle"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				req.WaitTitle = n
			}
		}
		if v := q.Get("timeout"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				req.Timeout = n
			}
		}
	} else {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	// Browser resolution: request > session > instance > global default > chrome
	requestBrowser := strings.TrimSpace(req.Browser)
	var sessionBrowser string
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		sessionBrowser = sess.Browser
	}
	if h.rejectBrowserConflictWithRunning(w, requestBrowser, sessionBrowser) {
		return
	}
	tabID := strings.TrimSpace(req.TabID)
	var instanceBrowser string
	if tabID != "" && h.Orchestrator != nil {
		if inst, ok := h.Orchestrator.FindInstanceByTab(tabID); ok && inst != nil && inst.Browser != "" {
			instanceBrowser = inst.Browser
		}
	}

	resolvedBrowser := config.ResolveBrowser(requestBrowser, sessionBrowser, instanceBrowser, h.Config.DefaultBrowser, h.Config.BrowsersAvailable)
	if resolvedBrowser != config.BrowserChrome {
		if _, err := config.ParseBrowser(resolvedBrowser, h.Config.BrowsersAvailable); err != nil {
			httpx.Error(w, http.StatusBadRequest, err)
			return
		}
	}

	// intentBrowser is the pre-downgrade resolution: the ownership conflict
	// check below must compare against what was asked for, not what the
	// DecisionSkip fallback executes on (ghost-chrome reads run on chrome).
	intentBrowser := resolvedBrowser
	handleDecision, err := checkBrowserCanHandle(resolvedBrowser, browsers.RequestIntent{
		Shape: browsers.ShapeRenderedRead,
	})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return
	}
	if handleDecision.Decision == browsers.DecisionSkip {
		resolvedBrowser = config.BrowserChrome
	}

	// Resolve the effective config with target-specific overrides (binary,
	// proxy, Cloak, extraFlags) merged in.
	effectiveCfg, err := h.resolveEffectiveConfig(resolvedBrowser)
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
		return
	}

	// With instance browser feeding resolution above, this only fires when an
	// explicit request/session browser disagrees with the owning instance.
	if intentBrowser != "" && tabID != "" && h.Orchestrator != nil {
		if inst, ok := h.Orchestrator.FindInstanceByTab(tabID); ok && inst != nil {
			if inst.Browser != "" && config.NormalizeBrowser(inst.Browser) != config.NormalizeBrowser(intentBrowser) {
				httpx.ErrorCode(w, http.StatusConflict, "browser_conflict",
					fmt.Sprintf("tab %q is owned by instance with browser %q; cannot navigate with browser %q",
						tabID, inst.Browser, intentBrowser),
					false, map[string]any{
						"tabId":            tabID,
						"instanceBrowser":  inst.Browser,
						"requestedBrowser": intentBrowser,
					})
				return
			}
		}
	}

	if err := validateNavigateURL(req.URL); err != nil {
		httpx.Error(w, 400, err)
		return
	}

	domainResult := h.IDPIGuard.CheckDomain(req.URL)
	if domainResult.Blocked {
		h.recordNavigateRequest(r, req.TabID, req.URL)
		httpx.Error(w, http.StatusForbidden, fmt.Errorf("navigation blocked by IDPI: %s", domainResult.Reason))
		return
	}
	if domainResult.Threat {
		w.Header().Set("X-IDPI-Warning", domainResult.Reason)
	}

	trustedResolveCIDRs := parseCIDRs(effectiveCfg.TrustedResolveCIDRs)
	target, err := validateNavigateTarget(req.URL, h.IDPIGuard.DomainAllowed(req.URL), trustedResolveCIDRs)
	if err != nil {
		httpx.Error(w, http.StatusForbidden, err)
		return
	}
	trustedCIDRs := buildNavigateTrustedProxyCIDRs(effectiveCfg)
	h.recordNavigateRequest(r, req.TabID, req.URL)

	navRoute := browserops.SingleBrowserRoute(resolvedBrowser)
	navRoute.Attempts = append(navRoute.Attempts, browserops.RouteAttempt{
		Browser:  resolvedBrowser,
		Accepted: handleDecision.Decision == browsers.DecisionHandle,
		Reason:   handleDecision.Reason,
	})
	if requestBrowser != "" {
		navRoute.RequestedBrowser = requestBrowser
	}
	h.recordActivity(r, activity.Update{Route: navRoute})

	explicitTabID := strings.TrimSpace(req.TabID) != ""
	scopedCurrentForNavigate := false
	identifiedCaller := !currentTabScopeFromRequest(r).IsGlobal()
	if !explicitTabID && !req.NewTab {
		if scopedTabID, ok := h.scopedCurrentTabForRequest(r); ok {
			req.TabID = scopedTabID
			scopedCurrentForNavigate = true
		} else if identifiedCaller && h.EmptyPointerPolicy() == EmptyPointerStrict {
			// Strict empty-pointer policy: identified callers must pin a
			// tab explicitly. Refuse to lazily create one.
			httpx.ErrorCode(w, http.StatusConflict, "no_current_tab",
				"no current tab; explicit tabId required under strict empty-pointer policy",
				false, nil)
			return
		}
	}

	// Default to creating new tab (API design: /navigate always creates new tab)
	// unless explicitly reusing an existing tab, or an identified caller has a
	// scoped current tab.
	if req.TabID == "" {
		req.NewTab = true
	}

	// Static-first phase: a ghost-chrome-capable bridge can serve a fresh
	// navigate from its static browser without launching Chrome at all
	// (parity with main's lite mode). Only new-tab navigates qualify —
	// existing tabs already imply a running browser. On escalation, the
	// Chrome path below runs with SkipStatic so the fetch isn't repeated.
	//
	// Timeout budget is PER PHASE, not shared: the static attempt gets
	// NavigateTimeout (default 30s), and on escalation the Chrome navigate
	// gets a fresh budget of its own — worst case a navigate takes up to
	// twice the configured timeout. Sharing one budget would let a slow
	// static fetch starve the Chrome attempt that exists to rescue it.
	skipStatic := false
	var staticEscRoute *browserops.RouteMetadata
	if sf, ok := h.Bridge.(staticFirstNavigator); ok && sf.StaticFirstNavigate() && req.NewTab {
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
			return
		}
		skipStatic = true
		var esc *bridge.StaticEscalateError
		if errors.As(navErr, &esc) && esc.Route != nil {
			staticEscRoute = esc.Route
		}
	}

	if !h.ensureBrowserOrRespond(w, effectiveCfg) {
		return
	}

	if staticEscRoute != nil {
		// Merge the static attempt with the Chrome escalation, mirroring the
		// adapter's internal-escalation metadata shape.
		staticEscRoute.Escalated = true
		staticEscRoute.Attempts = append(staticEscRoute.Attempts,
			browserops.RouteAttempt{Browser: "chrome", Accepted: true, Reason: "escalated"})
		navRoute = staticEscRoute
	}

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

	var blockPatterns []string

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

	if blockAds {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.AdBlockPatterns)
	}

	if blockMedia {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.MediaBlockPatterns)
	} else if blockImages {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.ImageBlockPatterns)
	}

	if req.NewTab {
		h.navigateNewTabBrowser(w, r, navigateBrowserOptions{
			URL:            req.URL,
			WaitFor:        req.WaitFor,
			WaitSelector:   req.WaitSelector,
			NavTimeout:     navTimeout,
			TitleWait:      titleWait,
			Target:         target,
			TrustedCIDRs:   trustedCIDRs,
			BlockPatterns:  blockPatterns,
			DismissBanners: req.DismissBanners,
			Route:          navRoute,
			MaxRedirects:   effectiveCfg.MaxRedirects,
			SkipStatic:     skipStatic,
		})
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		if scopedCurrentForNavigate {
			h.navigateNewTabBrowser(w, r, navigateBrowserOptions{
				URL:            req.URL,
				WaitFor:        req.WaitFor,
				WaitSelector:   req.WaitSelector,
				NavTimeout:     navTimeout,
				TitleWait:      titleWait,
				Target:         target,
				TrustedCIDRs:   trustedCIDRs,
				BlockPatterns:  blockPatterns,
				DismissBanners: req.DismissBanners,
				Route:          navRoute,
				MaxRedirects:   effectiveCfg.MaxRedirects,
			})
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
	navGuard, err := installNavigateRuntimeGuardWithBridge(h.Bridge, tCtx, tCancel, target, trustedCIDRs)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
		return
	}
	if len(blockPatterns) > 0 {
		_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
	} else {
		_ = bridge.SetResourceBlocking(tCtx, nil)
	}

	navResult, navErr := h.Bridge.Navigate(tCtx, req.URL, bridge.NavigateParams{
		MaxRedirects: effectiveCfg.MaxRedirects,
	})
	if navErr != nil {
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				httpx.Error(w, http.StatusForbidden, blockedErr)
				return
			}
		}
		code := 500
		errMsg := navErr.Error()
		if errors.Is(navErr, bridge.ErrTooManyRedirects) {
			code = 422
		} else if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
			code = 400
		}
		navigateErrorWithHint(w, code, navErr, req.URL)
		return
	}
	if navResult != nil && navResult.Route != nil {
		navRoute = navResult.Route
	}

	h.Bridge.DeleteRefCache(resolvedTabID)
	h.clearTabFrameScope(resolvedTabID)

	// The ghost-chrome adapter may serve a navigate from a static tab instead
	// of the tab this handler drove; its result is authoritative. The static
	// document is fully loaded by construction, so waitFor/waitSelector are
	// already satisfied, and the remaining post-steps are Chrome-tab CDP ops
	// that would run against a tab that never navigated.
	if navResult != nil && navResult.TabID != "" && navResult.TabID != resolvedTabID {
		h.setCurrentTabForRequest(r, navResult.TabID)
		h.recordResolvedURL(r, navResult.URL)
		httpx.JSON(w, 200, map[string]any{"tabId": navResult.TabID, "url": navResult.URL, "title": navResult.Title, "route": navRoute})
		return
	}

	if err := h.waitForNavigationState(tCtx, req.WaitFor, req.WaitSelector); err != nil {
		httpx.ErrorCode(w, 400, "bad_wait_for", err.Error(), false, nil)
		return
	}

	h.maybeAutoSolve(tCtx, resolvedTabID, autoSolverTriggerNavigate)
	h.dismissBanners(tCtx, resolvedTabID, req.DismissBanners)

	navURL, _ := h.Bridge.CurrentURL(tCtx)
	title, _ := bridge.WaitForTitle(tCtx, titleWait)
	h.setCurrentTabForRequest(r, resolvedTabID)
	h.recordResolvedURL(r, navURL)

	httpx.JSON(w, 200, map[string]any{"tabId": resolvedTabID, "url": navURL, "title": title, "route": navRoute})
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
	navGuard, err := installNavigateRuntimeGuardWithBridge(h.Bridge, tCtx, tCancel, opts.Target, opts.TrustedCIDRs)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
		return
	}

	if len(opts.BlockPatterns) > 0 {
		_ = bridge.SetResourceBlocking(tCtx, opts.BlockPatterns)
	}

	navResult, navErr := h.Bridge.Navigate(tCtx, opts.URL, bridge.NavigateParams{
		MaxRedirects: opts.MaxRedirects,
		SkipStatic:   opts.SkipStatic,
	})
	if navErr != nil {
		// The blank tab never carried the requested URL; keeping it around on
		// failure only burns a MaxTabs slot until eviction.
		_ = h.Bridge.CloseTab(newTabID)
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				httpx.Error(w, http.StatusForbidden, blockedErr)
				return
			}
		}
		code := 500
		errMsg := navErr.Error()
		if errors.Is(navErr, bridge.ErrTooManyRedirects) {
			code = 422
		} else if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
			code = 400
		}
		navigateErrorWithHint(w, code, navErr, opts.URL)
		return
	}
	if navResult != nil && navResult.Route != nil {
		opts.Route = navResult.Route
	}

	// Static-first path served the navigate from its own tab: respond with
	// that tab and drop the unused blank Chrome tab created above. See the
	// matching branch in HandleNavigate for why post-steps are skipped.
	if navResult != nil && navResult.TabID != "" && navResult.TabID != newTabID {
		_ = h.Bridge.CloseTab(newTabID)
		h.setCurrentTabForRequest(r, navResult.TabID)
		h.recordResolvedTab(r, navResult.TabID)
		h.recordResolvedURL(r, navResult.URL)
		resp := map[string]any{"tabId": navResult.TabID, "url": navResult.URL, "title": navResult.Title}
		if opts.Route != nil {
			resp["route"] = opts.Route
		}
		httpx.JSON(w, 200, resp)
		return
	}

	if err := h.waitForNavigationState(tCtx, opts.WaitFor, opts.WaitSelector); err != nil {
		httpx.ErrorCode(w, 400, "bad_wait_for", err.Error(), false, nil)
		return
	}

	h.maybeAutoSolve(tCtx, newTabID, autoSolverTriggerNavigate)
	h.dismissBanners(tCtx, newTabID, opts.DismissBanners)

	navURL, _ := h.Bridge.CurrentURL(tCtx)
	title, _ := bridge.WaitForTitle(tCtx, opts.TitleWait)
	h.setCurrentTabForRequest(r, newTabID)
	h.recordResolvedTab(r, newTabID)
	h.recordResolvedURL(r, navURL)

	resp := map[string]any{"tabId": newTabID, "url": navURL, "title": title}
	if opts.Route != nil {
		resp["route"] = opts.Route
	}
	httpx.JSON(w, 200, resp)
}

// HandleTabNavigate navigates an existing tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/navigate
func (h *Handlers) HandleTabNavigate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	if r.Body != nil {
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&body)
		if err != nil && !errors.Is(err, io.EOF) {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			httpx.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	// Path tab ID is canonical for this endpoint and always navigates existing tab.
	body["tabId"] = tabID
	body["newTab"] = false

	payload, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.HandleNavigate(w, req)
}

// binaryFileExtensions are extensions that Chrome cannot render and will abort on
var binaryFileExtensions = []string{".gz", ".zip", ".tar", ".rar", ".7z", ".bz2", ".xz", ".pdf", ".exe", ".bin", ".dmg", ".iso"}

// isNavigateAbortedOnBinary checks if a navigation error is ERR_ABORTED on a likely binary URL
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

// navigateErrorWithHint returns an error response with remedy hints for binary content
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
