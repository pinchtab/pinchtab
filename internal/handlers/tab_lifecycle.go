package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const tabActionNew = "new"

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
		httpx.Error(w, 400, fmt.Errorf("action must be 'new' or 'focus'; use /close to close a tab"))
	}
}

// HandleTabClose closes the tab identified by the path. It is the tab-scoped
// equivalent of POST /close and exists so orchestrator dashboard commands can
// use the common /tabs/{id}/... proxy path.
func (h *Handlers) HandleTabClose(w http.ResponseWriter, r *http.Request) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	if r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 {
		var req struct {
			TabID string `json:"tabId"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
		if req.TabID != "" && req.TabID != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	h.closeTab(w, r, tabID)
}

// HandleClose closes the tab identified by the JSON body, or the current/default
// tab when tabId is omitted. It is the shorthand form of POST /tabs/{id}/close.
func (h *Handlers) HandleClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.closeTab(w, r, strings.TrimSpace(req.TabID))
}

func (h *Handlers) closeTab(w http.ResponseWriter, r *http.Request, tabID string) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	if tabID == "" {
		_, resolvedTabID, err := h.tabContext(r, "")
		if err != nil {
			httpx.Error(w, 404, err)
			return
		}
		tabID = resolvedTabID
	}

	if err := h.Bridge.CloseTab(tabID); err != nil {
		httpx.Error(w, 500, err)
		return
	}

	h.recordActivity(r, activity.Update{Action: "tab.close", TabID: tabID})
	w.Header().Set(activity.HeaderPTTabID, tabID)

	httpx.JSON(w, 200, map[string]any{"closed": true, "tabId": tabID})
}
