package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
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
		TabID        string  `json:"tabId"`
		URL          string  `json:"url"`
		NewTab       bool    `json:"newTab"`
		WaitTitle    float64 `json:"waitTitle"`
		Timeout      float64 `json:"timeout"`
		BlockImages  *bool   `json:"blockImages"`
		BlockMedia   *bool   `json:"blockMedia"`
		BlockAds     *bool   `json:"blockAds"`
		WaitFor      string  `json:"waitFor"`
		WaitSelector string  `json:"waitSelector"`
	}

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.URL = q.Get("url")
		req.TabID = q.Get("tabId")
		req.NewTab = strings.EqualFold(q.Get("newTab"), "true") || q.Get("newTab") == "1"
		req.WaitFor = q.Get("waitFor")
		req.WaitSelector = q.Get("waitSelector")
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
	trustedCIDRs := parseCIDRs(h.Config.TrustedProxyCIDRs)
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

	// Ensure Chrome is initialized
	if !h.ensureChromeOrRespond(w) {
		return
	}

	// Default to creating new tab (API design: /navigate always creates new tab)
	// Unless explicitly reusing an existing tab by specifying TabID
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
		// Create a blank tab first so the requested URL becomes the first
		// real history entry.
		newTabID, newCtx, _, err := h.Bridge.CreateTab("")
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("new tab: %w", err))
			return
		}

		tCtx, tCancel := context.WithTimeout(newCtx, navTimeout)
		defer tCancel()
		go httpx.CancelOnClientDone(r.Context(), tCancel)
		navGuard, err := installNavigateRuntimeGuard(tCtx, tCancel, target, trustedCIDRs)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
			return
		}

		if len(blockPatterns) > 0 {
			_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
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

		if err := h.waitForNavigationState(tCtx, req.WaitFor, req.WaitSelector); err != nil {
			httpx.ErrorCode(w, 400, "bad_wait_for", err.Error(), false, nil)
			return
		}

		h.maybeAutoSolve(tCtx, newTabID, autoSolverTriggerNavigate)

		var navURL string
		_ = chromedp.Run(tCtx, chromedp.Location(&navURL))
		title, _ := bridge.WaitForTitle(tCtx, titleWait)
		h.recordResolvedTab(r, newTabID)
		h.recordResolvedURL(r, navURL)

		httpx.JSON(w, 200, map[string]any{"tabId": newTabID, "url": navURL, "title": title})
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

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
		// Clear any existing blocking patterns
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

	var navURL string
	_ = chromedp.Run(tCtx, chromedp.Location(&navURL))
	title, _ := bridge.WaitForTitle(tCtx, titleWait)
	h.recordResolvedURL(r, navURL)

	httpx.JSON(w, 200, map[string]any{"tabId": resolvedTabID, "url": navURL, "title": title})
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

const (
	tabActionNew   = "new"
	tabActionClose = "close"
)

func (h *Handlers) HandleTab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	switch req.Action {
	case tabActionNew:
		var target *validatedNavigateTarget
		trustedResolveCIDRs := parseCIDRs(h.Config.TrustedResolveCIDRs)
		trustedCIDRs := parseCIDRs(h.Config.TrustedProxyCIDRs)
		if req.URL != "" && req.URL != "about:blank" {
			if err := validateNavigateURL(req.URL); err != nil {
				httpx.Error(w, 400, err)
				return
			}
			domainResult := h.IDPIGuard.CheckDomain(req.URL)
			if domainResult.Blocked {
				httpx.Error(w, http.StatusForbidden, fmt.Errorf("navigation blocked by IDPI: %s", domainResult.Reason))
				return
			}
			if domainResult.Threat {
				w.Header().Set("X-IDPI-Warning", domainResult.Reason)
			}
			var err error
			target, err = validateNavigateTarget(req.URL, h.IDPIGuard.DomainAllowed(req.URL), trustedResolveCIDRs)
			if err != nil {
				httpx.Error(w, http.StatusForbidden, err)
				return
			}
		}

		if !h.ensureChromeOrRespond(w) {
			return
		}

		// Create a blank tab first so the requested URL becomes the first
		// real history entry.
		newTabID, ctx, _, err := h.Bridge.CreateTab("")
		if err != nil {
			httpx.Error(w, 500, err)
			return
		}

		if req.URL != "" && req.URL != "about:blank" {
			tCtx, tCancel := context.WithTimeout(ctx, h.Config.NavigateTimeout)
			defer tCancel()
			go httpx.CancelOnClientDone(r.Context(), tCancel)
			navGuard, err := installNavigateRuntimeGuard(tCtx, tCancel, target, trustedCIDRs)
			if err != nil {
				_ = h.Bridge.CloseTab(newTabID)
				httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
				return
			}
			if err := bridge.NavigatePageWithRedirectLimit(tCtx, req.URL, h.Config.MaxRedirects); err != nil {
				if navGuard != nil {
					if blockedErr := navGuard.blocked(); blockedErr != nil {
						_ = h.Bridge.CloseTab(newTabID)
						httpx.Error(w, http.StatusForbidden, blockedErr)
						return
					}
				}
				_ = h.Bridge.CloseTab(newTabID)
				code := 500
				if errors.Is(err, bridge.ErrTooManyRedirects) {
					code = 422
				}
				navigateErrorWithHint(w, code, err, req.URL)
				return
			}
		}

		var curURL, title string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL), chromedp.Title(&title))

		httpx.JSON(w, 200, map[string]any{"tabId": newTabID, "url": curURL, "title": title})

	case tabActionClose:
		if req.TabID == "" {
			httpx.Error(w, 400, fmt.Errorf("tabId required"))
			return
		}
		if !h.ensureChromeOrRespond(w) {
			return
		}

		if err := h.Bridge.CloseTab(req.TabID); err != nil {
			httpx.Error(w, 500, err)
			return
		}

		h.recordActivity(r, activity.Update{Action: "tab.close", TabID: req.TabID})

		httpx.JSON(w, 200, map[string]any{"closed": true})

	case "focus":
		if req.TabID == "" {
			httpx.Error(w, 400, fmt.Errorf("tabId required"))
			return
		}
		if !h.ensureChromeOrRespond(w) {
			return
		}
		if err := h.Bridge.FocusTab(req.TabID); err != nil {
			httpx.Error(w, 404, err)
			return
		}

		h.recordActivity(r, activity.Update{Action: "tab.focus", TabID: req.TabID})

		httpx.JSON(w, 200, map[string]any{"focused": true, "tabId": req.TabID})

	default:
		httpx.Error(w, 400, fmt.Errorf("action must be 'new', 'close', or 'focus'"))
	}
}

// HandleTabBack navigates a specific tab back in history.
func (h *Handlers) HandleTabBack(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("tabId", r.PathValue("id"))
	r.URL.RawQuery = q.Encode()
	h.HandleBack(w, r)
}

// HandleTabForward navigates a specific tab forward in history.
func (h *Handlers) HandleTabForward(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("tabId", r.PathValue("id"))
	r.URL.RawQuery = q.Encode()
	h.HandleForward(w, r)
}

// HandleTabReload reloads a specific tab.
func (h *Handlers) HandleTabReload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("tabId", r.PathValue("id"))
	r.URL.RawQuery = q.Encode()
	h.HandleReload(w, r)
}

// HandleBack navigates the current (or specified) tab back in history.
func (h *Handlers) HandleBack(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	// Use CDP directly instead of chromedp.NavigateBack() which wraps in
	// responseAction() and waits for Page.loadEventFired — hangs indefinitely.
	var noHistory bool
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cur, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}
		if cur <= 0 || cur > int64(len(entries)-1) {
			noHistory = true
			return nil
		}
		return page.NavigateToHistoryEntry(entries[cur-1].ID).Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("back: %w", err))
		return
	}
	if !noHistory {
		time.Sleep(200 * time.Millisecond)
	}

	var curURL string
	_ = chromedp.Run(ctx, chromedp.Location(&curURL))
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// HandleForward navigates the current (or specified) tab forward in history.
func (h *Handlers) HandleForward(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	// Use CDP directly instead of chromedp.NavigateForward() which wraps in
	// responseAction() and waits for Page.loadEventFired — hangs indefinitely.
	var noHistory bool
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cur, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}
		if cur < 0 || cur >= int64(len(entries)-1) {
			noHistory = true
			return nil
		}
		return page.NavigateToHistoryEntry(entries[cur+1].ID).Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("forward: %w", err))
		return
	}
	if !noHistory {
		time.Sleep(200 * time.Millisecond)
	}

	var curURL string
	_ = chromedp.Run(ctx, chromedp.Location(&curURL))
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
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

// HandleReload reloads the current (or specified) tab.
func (h *Handlers) HandleReload(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.Reload().Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("reload: %w", err))
		return
	}

	// Wait briefly for page to start loading
	var curURL string
	_ = chromedp.Run(ctx, chromedp.Location(&curURL))
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}
