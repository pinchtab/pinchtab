package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type routeRuleRequest struct {
	Pattern      string `json:"pattern"`
	Action       string `json:"action"`
	Body         string `json:"body,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	Status       int    `json:"status,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	Method       string `json:"method,omitempty"`
}

// HandleNetworkRoute installs (or replaces by pattern) an interception rule on
// the active or query-specified tab.
//
// @Endpoint POST /network/route
// @Description Install a request interception rule on a tab
//
// @Param tabId   string query Tab ID (optional, uses current tab if empty)
// @Param body    object body  {pattern, action, body?, contentType?, status?, resourceType?}
//
// @Response 200 application/json {ok, rules}
// @Response 400 application/json Validation error
// @Response 404 application/json Tab not found
func (h *Handlers) HandleNetworkRoute(w http.ResponseWriter, r *http.Request) {
	h.handleNetworkRouteFor(w, r, r.URL.Query().Get("tabId"))
}

// HandleNetworkUnroute removes one or all rules.
//
// @Endpoint DELETE /network/route
// @Description Remove an interception rule (or all if pattern is empty/missing)
//
// @Param tabId   string query Tab ID (optional, uses current tab if empty)
// @Param pattern string query Pattern to remove (omit to remove all)
//
// @Response 200 application/json {ok, removed, rules}
// @Response 404 application/json Tab not found
func (h *Handlers) HandleNetworkUnroute(w http.ResponseWriter, r *http.Request) {
	h.handleNetworkUnrouteFor(w, r, r.URL.Query().Get("tabId"))
}

// HandleNetworkRouteList returns the current rules for the active or
// query-specified tab.
//
// @Endpoint GET /network/route
// @Description List active interception rules for a tab
func (h *Handlers) HandleNetworkRouteList(w http.ResponseWriter, r *http.Request) {
	h.handleNetworkRouteListFor(w, r, r.URL.Query().Get("tabId"))
}

// HandleTabNetworkRoute is the path-scoped wrapper for HandleNetworkRoute.
//
// @Endpoint POST /tabs/{id}/network/route
func (h *Handlers) HandleTabNetworkRoute(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	h.handleNetworkRouteFor(w, r, tabID)
}

// HandleTabNetworkUnroute is the path-scoped wrapper for HandleNetworkUnroute.
//
// @Endpoint DELETE /tabs/{id}/network/route
func (h *Handlers) HandleTabNetworkUnroute(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	h.handleNetworkUnrouteFor(w, r, tabID)
}

// HandleTabNetworkRouteList is the path-scoped wrapper for HandleNetworkRouteList.
//
// @Endpoint GET /tabs/{id}/network/route
func (h *Handlers) HandleTabNetworkRouteList(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	h.handleNetworkRouteListFor(w, r, tabID)
}

// requireRouteContext is the shared prelude for the network/route handlers.
// It runs the capability gate, ensures Chrome is up, and resolves the tab
// context. On failure the response has been written and ok=false. On success
// the caller has tabCtx + resolved tab ID; mutation handlers should follow up
// with enforceCurrentTabDomainPolicy(tabCtx, resolvedID).
func (h *Handlers) requireRouteContext(w http.ResponseWriter, r *http.Request, tabID string) (tabCtx context.Context, resolvedID string, ok bool) {
	if !h.networkInterceptEnabled() {
		httpx.ErrorCode(w, 403, "network_intercept_disabled",
			httpx.DisabledEndpointMessage("networkIntercept", "security.allowNetworkIntercept"),
			false, map[string]any{"setting": "security.allowNetworkIntercept"})
		return nil, "", false
	}
	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return nil, "", false
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return nil, "", false
	}
	ctx, id, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return nil, "", false
	}
	return ctx, id, true
}

func (h *Handlers) handleNetworkRouteFor(w http.ResponseWriter, r *http.Request, tabID string) {
	tabCtx, resolvedID, ok := h.requireRouteContext(w, r, tabID)
	if !ok {
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, tabCtx, resolvedID); !ok {
		return
	}

	var req routeRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode body: %w", err))
		return
	}
	if req.Pattern == "" {
		httpx.Error(w, 400, fmt.Errorf("pattern required"))
		return
	}
	// Early validation for clean 400s. AddRule re-checks authoritatively so
	// non-HTTP callers are protected too — these are belt-and-suspenders.
	if len(req.Body) > bridge.MaxFulfillBodyBytes {
		httpx.Error(w, 400, fmt.Errorf("body exceeds %d bytes (cap)", bridge.MaxFulfillBodyBytes))
		return
	}
	if req.Status != 0 && (req.Status < 100 || req.Status > 599) {
		httpx.Error(w, 400, fmt.Errorf("status %d out of HTTP range (100-599)", req.Status))
		return
	}
	if req.ContentType != "" && !bridge.IsFulfillContentTypeAllowed(req.ContentType) {
		httpx.Error(w, 400, fmt.Errorf("contentType %q is not on the fulfill safe-list (or contains control chars)", req.ContentType))
		return
	}
	if !bridge.IsResourceTypeValid(req.ResourceType) {
		httpx.Error(w, 400, fmt.Errorf("invalid resourceType %q", req.ResourceType))
		return
	}
	if req.Action == "" {
		req.Action = string(bridge.RouteActionContinue)
	}

	rule := bridge.RouteRule{
		Pattern:      req.Pattern,
		Action:       bridge.RouteAction(req.Action),
		Body:         req.Body,
		ContentType:  req.ContentType,
		Status:       req.Status,
		ResourceType: req.ResourceType,
		Method:       req.Method,
	}
	if err := h.Bridge.AddRouteRule(resolvedID, rule); err != nil {
		httpx.Error(w, 400, err)
		return
	}

	rules, _ := h.Bridge.ListRouteRules(resolvedID)
	httpx.JSON(w, 200, map[string]any{
		"ok":    true,
		"tabId": resolvedID,
		"rules": rules,
	})
}

func (h *Handlers) handleNetworkUnrouteFor(w http.ResponseWriter, r *http.Request, tabID string) {
	tabCtx, resolvedID, ok := h.requireRouteContext(w, r, tabID)
	if !ok {
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, tabCtx, resolvedID); !ok {
		return
	}

	pattern := r.URL.Query().Get("pattern")
	if pattern == "" && r.ContentLength > 0 {
		var body struct {
			Pattern string `json:"pattern"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode body: %w", err))
			return
		}
		pattern = body.Pattern
	}

	removed, err := h.Bridge.RemoveRouteRule(resolvedID, pattern)
	if err != nil {
		if errors.Is(err, bridge.ErrTabNotRouted) {
			WriteTabContextError(w, err, 404)
			return
		}
		httpx.Error(w, 500, err)
		return
	}
	rules, _ := h.Bridge.ListRouteRules(resolvedID)
	httpx.JSON(w, 200, map[string]any{
		"ok":      true,
		"tabId":   resolvedID,
		"removed": removed,
		"rules":   rules,
	})
}

func (h *Handlers) handleNetworkRouteListFor(w http.ResponseWriter, r *http.Request, tabID string) {
	_, resolvedID, ok := h.requireRouteContext(w, r, tabID)
	if !ok {
		return
	}
	rules, err := h.Bridge.ListRouteRules(resolvedID)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	if rules == nil {
		rules = []bridge.RouteRule{}
	}
	httpx.JSON(w, 200, map[string]any{
		"tabId": resolvedID,
		"rules": rules,
	})
}
