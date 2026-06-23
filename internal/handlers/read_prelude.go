package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// resolveReadRouting performs the shared browser routing for a rendered/static
// read: it resolves the browser, records the read request, builds the route
// metadata (with the handle attempt and any requested browser), and records the
// route activity. ok=false means an error response was already written. The
// returned route is threaded back to the caller for the eventual response.
func (h *Handlers) resolveReadRouting(w http.ResponseWriter, r *http.Request, tabID, recordName, shape string) (*config.RuntimeConfig, *browserops.RouteMetadata, bool) {
	routing, ok := h.resolveBrowserForRequest(w, r, tabID, strings.TrimSpace(r.URL.Query().Get("browser")), browsers.RequestIntent{
		Shape: shape,
	})
	if !ok {
		return nil, nil, false
	}

	h.recordReadRequest(r, recordName, tabID)

	route := browserops.SingleBrowserRoute(routing.Browser)
	route.Attempts = append(route.Attempts, browserops.RouteAttempt{
		Browser:  routing.Browser,
		Accepted: routing.Decision.Decision == browsers.DecisionHandle,
		Reason:   routing.Decision.Reason,
	})
	if routing.RequestBrowser != "" {
		route.RequestedBrowser = routing.RequestBrowser
	}
	h.recordActivity(r, activity.Update{Route: route})

	return routing.EffectiveCfg, route, true
}

// resolveReadContext resolves the tab context, enforces the current-tab domain
// policy, and wires the action-timeout context (launching the client-disconnect
// canceller). ok=false means an error response was already written.
//
// When ok, the CALLER must defer in this registration order so the prior LIFO
// semantics are preserved (cancel runs before auto-close arming):
//
//	defer h.armAutoCloseIfEnabled(resolvedTabID)
//	defer cancel()
func (h *Handlers) resolveReadContext(w http.ResponseWriter, r *http.Request, tabID string, actionTimeout time.Duration) (resolvedTabID string, tCtx context.Context, cancel context.CancelFunc, ok bool) {
	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return "", nil, nil, false
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return "", nil, nil, false
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	go httpx.CancelOnClientDone(r.Context(), tCancel)
	return resolvedTabID, tCtx, tCancel, true
}

// resolveBinaryReadContext is the visual-export sibling of resolveReadContext: it
// resolves the tab context (no request header), enforces the current-tab domain
// policy, and wires the action-timeout/client-cancel — but does NOT arm
// auto-close (screenshot/pdf are one-shot exports). ok=false means an error
// response was already written. The caller must defer cancel().
func (h *Handlers) resolveBinaryReadContext(w http.ResponseWriter, r *http.Request, tabID string, actionTimeout time.Duration) (resolvedTabID string, tCtx context.Context, cancel context.CancelFunc, ok bool) {
	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return "", nil, nil, false
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return "", nil, nil, false
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	go httpx.CancelOnClientDone(r.Context(), tCancel)
	return resolvedTabID, tCtx, tCancel, true
}

// resolveTargetFrameID returns the explicit ?frameId=; when absent it falls back
// to the currently-scoped frame on the tab (as set by /frame). Empty means the
// top-level document.
func (h *Handlers) resolveTargetFrameID(r *http.Request, resolvedTabID string) string {
	targetFrameID := r.URL.Query().Get("frameId")
	if targetFrameID == "" {
		if scope, ok := h.currentFrameScope(resolvedTabID); ok {
			targetFrameID = scope.FrameID
		}
	}
	return targetFrameID
}
