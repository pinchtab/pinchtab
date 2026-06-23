package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

func (h *Handlers) ensureCookiesEnabled(w http.ResponseWriter) bool {
	if h.cookiesEnabled() {
		return true
	}
	h.writeCapabilityDisabled(w, routes.CapCookies)
	return false
}

func (h *Handlers) HandleGetCookies(w http.ResponseWriter, r *http.Request) {
	if !h.ensureCookiesEnabled(w) {
		return
	}

	tabID := r.URL.Query().Get("tabId")
	url := r.URL.Query().Get("url")
	name := r.URL.Query().Get("name")

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	if url != "" && !h.enforceURLDomainPolicy(w, url) {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	if url == "" {
		url, _ = h.Bridge.CurrentURL(tCtx)
	}

	cookies, err := h.Bridge.GetCookies(tCtx, []string{url})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("get cookies: %w", err))
		return
	}

	if name != "" {
		filtered := make([]bridge.CookieData, 0)
		for _, c := range cookies {
			if c.Name == name {
				filtered = append(filtered, c)
			}
		}
		cookies = filtered
	}

	h.recordActivity(r, activity.Update{Action: "cookies.read"})

	result := make([]map[string]any, len(cookies))
	for i, c := range cookies {
		result[i] = map[string]any{
			"name":     c.Name,
			"value":    c.Value,
			"domain":   c.Domain,
			"path":     c.Path,
			"secure":   c.Secure,
			"httpOnly": c.HTTPOnly,
			"sameSite": c.SameSite,
		}
		if c.Expires > 0 {
			result[i]["expires"] = c.Expires
		}
	}

	httpx.JSON(w, 200, map[string]any{
		"url":     url,
		"cookies": result,
		"count":   len(result),
	})
}

// HandleTabGetCookies gets cookies for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/cookies
func (h *Handlers) HandleTabGetCookies(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleGetCookies)
}

type cookieRequest struct {
	TabID   string             `json:"tabId"`
	URL     string             `json:"url"`
	Cookies []cookieSetRequest `json:"cookies"`
}

type cookieSetRequest struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
	Expires  float64 `json:"expires"`
}

// HandleClearCookies clears all browser cookies.
//
// @Endpoint DELETE /cookies
func (h *Handlers) HandleClearCookies(w http.ResponseWriter, r *http.Request) {
	if !h.ensureCookiesEnabled(w) {
		return
	}

	if err := h.ensureBrowser(h.Config); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, err)
		return
	}

	ctx := h.Bridge.BrowserContext()
	if err := h.Bridge.ClearCookies(ctx); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("clear cookies: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "cookies.clear"})

	httpx.JSON(w, http.StatusOK, map[string]any{"status": "cleared"})
}

// HandleTabClearCookies clears all browser cookies (tab-scoped variant for API consistency).
//
// @Endpoint DELETE /tabs/{id}/cookies
func (h *Handlers) HandleTabClearCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	if !h.ensureCookiesEnabled(w) {
		return
	}

	// Verify tab exists before clearing.
	if _, _, err := h.tabContext(r, tabID); err != nil {
		WriteTabContextError(w, err, 404)
		return
	}

	h.HandleClearCookies(w, r)
}

func (h *Handlers) HandleSetCookies(w http.ResponseWriter, r *http.Request) {
	if !h.ensureCookiesEnabled(w) {
		return
	}

	var req cookieRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.URL == "" {
		httpx.Error(w, 400, fmt.Errorf("url is required"))
		return
	}

	if len(req.Cookies) == 0 {
		httpx.Error(w, 400, fmt.Errorf("cookies array is empty"))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	if !h.enforceURLDomainPolicy(w, req.URL) {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	successCount := 0
	for _, cookie := range req.Cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}

		if err := h.Bridge.SetCookie(tCtx, bridge.SetCookieParams{
			Name:     cookie.Name,
			Value:    cookie.Value,
			URL:      req.URL,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HTTPOnly,
			SameSite: cookie.SameSite,
			Expires:  cookie.Expires,
		}); err == nil {
			successCount++
		}
	}

	h.recordActivity(r, activity.Update{Action: "cookies.write"})

	httpx.JSON(w, 200, map[string]any{
		"set":    successCount,
		"failed": len(req.Cookies) - successCount,
		"total":  len(req.Cookies),
	})
}

// HandleTabSetCookies sets cookies for a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/cookies
func (h *Handlers) HandleTabSetCookies(w http.ResponseWriter, r *http.Request) {
	// Path id is canonical; reject a conflicting body tabId and forward to the
	// root handler, which re-decodes the cookieRequest.
	h.withPathTabIDBody(w, r, h.HandleSetCookies)
}
