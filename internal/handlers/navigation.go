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
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/httpx"
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
	}

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.URL = q.Get("url")
		req.TabID = q.Get("tabId")
		req.NewTab = strings.EqualFold(q.Get("newTab"), "true") || q.Get("newTab") == "1"
		req.WaitFor = q.Get("waitFor")
		req.WaitSelector = q.Get("waitSelector")
		req.DismissBanners = strings.EqualFold(q.Get("dismissBanners"), "true") || q.Get("dismissBanners") == "1"
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

	trustedResolveCIDRs := parseCIDRs(h.Config.TrustedResolveCIDRs)
	target, err := validateNavigateTarget(req.URL, h.IDPIGuard.DomainAllowed(req.URL), trustedResolveCIDRs)
	if err != nil {
		httpx.Error(w, http.StatusForbidden, err)
		return
	}
	trustedCIDRs := buildNavigateTrustedProxyCIDRs(h.Config)
	h.recordNavigateRequest(r, req.TabID, req.URL)

	// --- Lite engine fast path ---
	if h.useLite(engine.CapNavigate, req.URL) {
		h.recordEngine(r, "lite")
		var trustedResolvedIP []netip.Addr
		allowInternal := false
		if target != nil {
			trustedResolvedIP = target.trustedResolvedIP
			allowInternal = target.allowInternal
		}
		liteCtx := engine.WithNavigateNetworkPolicy(r.Context(), &engine.NavigateNetworkPolicy{
			AllowInternal:     allowInternal,
			TrustedProxyCIDRs: trustedCIDRs,
			TrustedResolvedIP: trustedResolvedIP,
			MaxRedirects:      h.Config.MaxRedirects,
		})
		result, err := h.Router.Lite().Navigate(liteCtx, req.URL)
		if err != nil {
			if engine.IsIDPIBlocked(err) || engine.IsNetworkPolicyBlocked(err) {
				httpx.Error(w, http.StatusForbidden, err)
			} else {
				httpx.Error(w, 502, fmt.Errorf("lite navigate: %w", err))
			}
			return
		}
		w.Header().Set("X-Engine", "lite")
		httpx.JSON(w, 200, map[string]any{"tabId": result.TabID, "url": result.URL, "title": result.Title})
		return
	}

	h.recordEngine(r, "chrome")
	w.Header().Set("X-Engine", "chrome")

	if !h.ensureChromeOrRespond(w) {
		return
	}

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

	titleWait := time.Duration(0)
	if req.WaitTitle > 0 {
		if req.WaitTitle > 30 {
			req.WaitTitle = 30
		}
		titleWait = time.Duration(req.WaitTitle * float64(time.Second))
	}

	navTimeout := h.Config.NavigateTimeout
	if req.Timeout > 0 {
		if req.Timeout > 120 {
			req.Timeout = 120
		}
		navTimeout = time.Duration(req.Timeout * float64(time.Second))
	}

	var blockPatterns []string

	blockAds := h.Config.BlockAds
	if req.BlockAds != nil {
		blockAds = *req.BlockAds
	}

	blockMedia := h.Config.BlockMedia
	if req.BlockMedia != nil {
		blockMedia = *req.BlockMedia
	}

	blockImages := h.Config.BlockImages
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
		h.navigateNewTabChrome(w, r, navigateChromeOptions{
			URL:            req.URL,
			WaitFor:        req.WaitFor,
			WaitSelector:   req.WaitSelector,
			NavTimeout:     navTimeout,
			TitleWait:      titleWait,
			Target:         target,
			TrustedCIDRs:   trustedCIDRs,
			BlockPatterns:  blockPatterns,
			DismissBanners: req.DismissBanners,
		})
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		if scopedCurrentForNavigate {
			h.navigateNewTabChrome(w, r, navigateChromeOptions{
				URL:            req.URL,
				WaitFor:        req.WaitFor,
				WaitSelector:   req.WaitSelector,
				NavTimeout:     navTimeout,
				TitleWait:      titleWait,
				Target:         target,
				TrustedCIDRs:   trustedCIDRs,
				BlockPatterns:  blockPatterns,
				DismissBanners: req.DismissBanners,
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
	navGuard, err := installNavigateRuntimeGuard(tCtx, tCancel, target, trustedCIDRs)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
		return
	}
	if len(blockPatterns) > 0 {
		_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
	} else {
		_ = bridge.SetResourceBlocking(tCtx, nil)
	}

	if err := bridge.NavigatePageWithRedirectLimit(tCtx, req.URL, h.Config.MaxRedirects); err != nil {
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				httpx.Error(w, http.StatusForbidden, blockedErr)
				return
			}
		}
		code := 500
		errMsg := err.Error()
		if errors.Is(err, bridge.ErrTooManyRedirects) {
			code = 422
		} else if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
			code = 400
		}
		navigateErrorWithHint(w, code, err, req.URL)
		return
	}

	h.Bridge.DeleteRefCache(resolvedTabID)

	if err := h.waitForNavigationState(tCtx, req.WaitFor, req.WaitSelector); err != nil {
		httpx.ErrorCode(w, 400, "bad_wait_for", err.Error(), false, nil)
		return
	}

	h.maybeAutoSolve(tCtx, resolvedTabID, autoSolverTriggerNavigate)
	h.dismissBanners(tCtx, resolvedTabID, req.DismissBanners)

	var navURL string
	_ = chromedp.Run(tCtx, chromedp.Location(&navURL))
	title, _ := bridge.WaitForTitle(tCtx, titleWait)
	h.setCurrentTabForRequest(r, resolvedTabID)
	h.recordResolvedURL(r, navURL)

	httpx.JSON(w, 200, map[string]any{"tabId": resolvedTabID, "url": navURL, "title": title})
}

type navigateChromeOptions struct {
	URL            string
	WaitFor        string
	WaitSelector   string
	NavTimeout     time.Duration
	TitleWait      time.Duration
	Target         *validatedNavigateTarget
	TrustedCIDRs   []*net.IPNet
	BlockPatterns  []string
	DismissBanners bool
}

func (h *Handlers) navigateNewTabChrome(w http.ResponseWriter, r *http.Request, opts navigateChromeOptions) {
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
	navGuard, err := installNavigateRuntimeGuard(tCtx, tCancel, opts.Target, opts.TrustedCIDRs)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
		return
	}

	if len(opts.BlockPatterns) > 0 {
		_ = bridge.SetResourceBlocking(tCtx, opts.BlockPatterns)
	}

	if err := bridge.NavigatePageWithRedirectLimit(tCtx, opts.URL, h.Config.MaxRedirects); err != nil {
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				httpx.Error(w, http.StatusForbidden, blockedErr)
				return
			}
		}
		code := 500
		errMsg := err.Error()
		if errors.Is(err, bridge.ErrTooManyRedirects) {
			code = 422
		} else if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
			code = 400
		}
		navigateErrorWithHint(w, code, err, opts.URL)
		return
	}

	if err := h.waitForNavigationState(tCtx, opts.WaitFor, opts.WaitSelector); err != nil {
		httpx.ErrorCode(w, 400, "bad_wait_for", err.Error(), false, nil)
		return
	}

	h.maybeAutoSolve(tCtx, newTabID, autoSolverTriggerNavigate)
	h.dismissBanners(tCtx, newTabID, opts.DismissBanners)

	var navURL string
	_ = chromedp.Run(tCtx, chromedp.Location(&navURL))
	title, _ := bridge.WaitForTitle(tCtx, opts.TitleWait)
	h.setCurrentTabForRequest(r, newTabID)
	h.recordResolvedTab(r, newTabID)
	h.recordResolvedURL(r, navURL)

	httpx.JSON(w, 200, map[string]any{"tabId": newTabID, "url": navURL, "title": title})
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
