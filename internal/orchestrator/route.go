package orchestrator

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
)

// RoutingDecision identifies which precedence rule selected the target
// instance for a shorthand request. Used for metrics and logging.
type RoutingDecision string

const (
	RoutingDecisionTabOwner RoutingDecision = "tab_owner"
	RoutingDecisionSession  RoutingDecision = "session"
	RoutingDecisionAgent    RoutingDecision = "agent"
	RoutingDecisionFallback RoutingDecision = "fallback"
)

const routeInstanceReadyWait = 30 * time.Second

var routeInstanceReadyPollInterval = 500 * time.Millisecond

// RouteForRequest resolves a shorthand request to a running instance URL,
// launching the selected target when none is available. This centralizes the
// target-aware auto-launch behavior used by strategies such as simple.
func (o *Orchestrator) RouteForRequest(r *http.Request) (string, int, error) {
	if o == nil {
		return "", http.StatusServiceUnavailable, fmt.Errorf("no orchestrator configured")
	}

	requestedTarget := ExtractRequestedBrowserTarget(r)
	if requestedTarget != "" {
		resolvedTarget, _, err := o.ResolveRequestedBrowserTarget(requestedTarget)
		if err != nil {
			return "", statusForRouteLaunchSelectionError(err), err
		}
		if target := o.FirstRunningURLForBrowserTarget(resolvedTarget); target != "" {
			return target, 0, nil
		}
		return o.launchAndWaitForRequestRoute(autoLaunchProfileName(resolvedTarget), requestedTarget)
	}

	target, status, err := o.FirstRunningURLForRequest(r)
	if err != nil {
		return "", status, err
	}
	if target != "" {
		return target, 0, nil
	}

	return o.launchAndWaitForRequestRoute("default", "")
}

func (o *Orchestrator) launchAndWaitForRequestRoute(profileName, requestedTarget string) (string, int, error) {
	slog.Info("request route: no running instance, auto-launching", "profile", profileName, "browserTarget", requestedTarget)
	launched, err := o.LaunchWithTargetSelection(profileName, "", true, requestedTarget, nil, LaunchOptions{})
	if err != nil {
		status := statusForRouteLaunchSelectionError(err)
		return "", status, fmt.Errorf("auto-launch failed: %w", err)
	}

	if launched.URL == "" {
		return "", http.StatusServiceUnavailable, fmt.Errorf("auto-launched instance %q has no URL", launched.ID)
	}
	url, err := o.waitForRequestRouteReady(launched.URL, routeInstanceReadyWait)
	if err != nil {
		return "", http.StatusServiceUnavailable, err
	}
	return url, 0, nil
}

func (o *Orchestrator) waitForRequestRouteReady(url string, timeout time.Duration) (string, error) {
	healthURL := strings.TrimRight(url, "/") + "/health"
	deadline := time.Now().Add(timeout)
	client := o.client
	if client == nil {
		client = http.DefaultClient
	}

	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, healthURL, nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return url, nil
			}
		}
		time.Sleep(routeInstanceReadyPollInterval)
	}

	return "", fmt.Errorf("instance launched but did not become ready in time")
}

func autoLaunchProfileName(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return "default"
	}
	return "default-" + target
}

func statusForRouteLaunchSelectionError(err error) int {
	if errors.Is(err, ErrUnknownBrowserTarget) {
		return http.StatusBadRequest
	}
	var exhausted *FallbackExhaustedError
	if errors.As(err, &exhausted) {
		return http.StatusBadGateway
	}
	return http.StatusServiceUnavailable
}

// WrapShorthand wraps a strategy-supplied fallback handler with the routing
// precedence defined in tab-spec.md:
//
//  1. Explicit tab id (path / query / JSON body): route to the owning
//     instance. Not found + multiple instances → 404; not found + single
//     instance → fall through to the only instance for legacy ergonomics.
//  2. Session binding: route to the bound instance if still running.
//  3. Agent binding: route to the bound instance if still running.
//  4. Fallback: invoke the strategy handler.
//
// The wrapper does not write or move bindings — that is step 1.5's job and
// happens in the proxy response hook after a successful response.
func (o *Orchestrator) WrapShorthand(fallback http.HandlerFunc) http.HandlerFunc {
	if o == nil {
		return fallback
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if tabID, _ := ExtractExplicitTabID(r); tabID != "" {
			if o.routeByTabOwner(w, r, tabID) {
				return
			}
			// routeByTabOwner already wrote a 404 / single-instance proxy;
			// only return false when no decision could be made.
			return
		}

		if instanceID, decision, ok := o.resolveIdentityBinding(r); ok {
			if o.routeToInstanceID(w, r, instanceID, decision) {
				return
			}
		}

		fallback(w, r)
	}
}

// routeByTabOwner handles precedence rule 1. Returns true if a response was
// already written (either a successful proxy or an error).
func (o *Orchestrator) routeByTabOwner(w http.ResponseWriter, r *http.Request, tabID string) bool {
	requestedTarget, status, err := o.requestedBrowserTargetForRoute(r)
	if err != nil {
		httpx.Error(w, status, err)
		return true
	}

	// Fast path via locator cache.
	if o.instanceMgr != nil {
		if inst, err := o.instanceMgr.FindInstanceByTabID(tabID); err == nil && inst != nil {
			if writeBrowserTargetConflict(w, tabID, inst, requestedTarget) {
				return true
			}
			if !o.allowCrossInstance(w, r, inst.ID) {
				return true
			}
			o.proxyToInstanceForRoute(w, r, inst, tabID, RoutingDecisionTabOwner)
			return true
		}
	}
	// Slow path: enumerate running instances.
	if internal, err := o.findRunningInstanceByTabID(tabID); err == nil && internal != nil {
		if o.instanceMgr != nil {
			o.instanceMgr.Locator.Register(tabID, internal.ID)
		}
		if writeBrowserTargetConflict(w, tabID, &internal.Instance, requestedTarget) {
			return true
		}
		if !o.allowCrossInstance(w, r, internal.ID) {
			return true
		}
		o.proxyToInstanceForRoute(w, r, &internal.Instance, tabID, RoutingDecisionTabOwner)
		return true
	}

	// Tab not found. If exactly one instance is running, fall through to it
	// — preserves legacy ergonomics for users running `--tab` against a
	// just-created tab whose id has not propagated to the dashboard yet.
	if only := o.singleRunningInstance(); only != nil {
		if writeBrowserTargetConflict(w, tabID, &only.Instance, requestedTarget) {
			return true
		}
		o.proxyToInstanceForRoute(w, r, &only.Instance, tabID, RoutingDecisionFallback)
		return true
	}

	// Multiple instances and no owner found — refuse to guess.
	httpx.Error(w, http.StatusNotFound, fmt.Errorf("tab %q not found", tabID))
	return true
}

func (o *Orchestrator) requestedBrowserTargetForRoute(r *http.Request) (string, int, error) {
	requested := ExtractRequestedBrowserTarget(r)
	if requested == "" {
		return "", 0, nil
	}
	resolved, err := config.ResolveExplicitBrowserTarget(o.runtimeCfg, requested)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	return resolved.Name, 0, nil
}

func writeBrowserTargetConflict(w http.ResponseWriter, tabID string, inst *bridge.Instance, requestedTarget string) bool {
	if requestedTarget == "" || inst == nil || inst.BrowserTarget == "" || inst.BrowserTarget == requestedTarget {
		return false
	}
	detail := fmt.Sprintf("instance %q has browserTarget %q; cannot route with browserTarget %q",
		inst.ID, inst.BrowserTarget, requestedTarget)
	if tabID != "" {
		detail = fmt.Sprintf("tab %q is owned by instance with browserTarget %q; cannot route with browserTarget %q",
			tabID, inst.BrowserTarget, requestedTarget)
	}
	meta := map[string]any{
		"instanceId":       inst.ID,
		"instanceTarget":   inst.BrowserTarget,
		"requestedTarget":  requestedTarget,
		"instanceProvider": inst.BrowserProvider,
	}
	if tabID != "" {
		meta["tabId"] = tabID
	}
	httpx.ErrorCode(w, http.StatusConflict, "browser_target_conflict",
		detail,
		false, meta)
	return true
}

// resolveIdentityBinding implements precedence rules 2 and 3. Public
// session-authenticated requests resolve from the authenticated session
// context. Trusted internal proxy hops resolve from the propagated session
// header. Raw public X-PinchTab-Session-Id is never trusted.
func (o *Orchestrator) resolveIdentityBinding(r *http.Request) (string, RoutingDecision, bool) {
	if r == nil || o.bindings == nil {
		return "", "", false
	}
	if id := sessionIDForRouting(r); id != "" {
		if inst, ok := o.bindings.ResolveSession(id); ok {
			return inst, RoutingDecisionSession, true
		}
	}
	if id := strings.TrimSpace(r.Header.Get(activity.HeaderAgentID)); id != "" {
		if inst, ok := o.bindings.ResolveAgent(id); ok {
			return inst, RoutingDecisionAgent, true
		}
	}
	return "", "", false
}

func sessionIDForRouting(r *http.Request) string {
	if r == nil {
		return ""
	}
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		if id := strings.TrimSpace(sess.ID); id != "" {
			return id
		}
	}
	if handlers.IsTrustedInternalProxy(r) {
		return strings.TrimSpace(r.Header.Get(activity.HeaderPTSessionID))
	}
	return ""
}

// routeToInstanceID validates the bound instance is still running, then
// proxies. Stale bindings (instance gone) are cleared and a fall-through
// signal is returned (false means "let fallback handle it").
func (o *Orchestrator) routeToInstanceID(w http.ResponseWriter, r *http.Request, instanceID string, decision RoutingDecision) bool {
	o.mu.RLock()
	internal, ok := o.instances[instanceID]
	o.mu.RUnlock()
	if !ok || internal == nil || internal.Status != "running" || !instanceIsActive(internal) {
		// Stale binding: clear and let fallback decide.
		if o.bindings != nil {
			o.bindings.ClearInstance(instanceID)
		}
		return false
	}
	requestedTarget, status, err := o.requestedBrowserTargetForRoute(r)
	if err != nil {
		httpx.Error(w, status, err)
		return true
	}
	if requestedTarget != "" {
		if internal.BrowserTarget == "" || internal.BrowserTarget != requestedTarget {
			return false
		}
	}
	o.proxyToInstanceForRoute(w, r, &internal.Instance, "", decision)
	return true
}

// allowCrossInstance decides whether a tab-owner-routed request is allowed
// to land on ownerID when the caller's identity is currently bound to a
// different instance. Returns false (and writes a 409) only when strict
// cross-instance routing is enabled. The default rule rebinds silently;
// the actual rebind happens in the proxy response hook on success.
func (o *Orchestrator) allowCrossInstance(w http.ResponseWriter, r *http.Request, ownerID string) bool {
	o.mu.RLock()
	strict := o.strictCrossInstanceTab
	o.mu.RUnlock()
	if !strict || o.bindings == nil || ownerID == "" {
		return true
	}
	if id := sessionIDForRouting(r); id != "" {
		if existing, ok := o.bindings.ResolveSession(id); ok && existing != ownerID {
			httpx.ErrorCode(w, http.StatusConflict, "cross_instance_tab",
				fmt.Sprintf("session %q is bound to instance %q; tab lives on %q", id, existing, ownerID),
				false, nil)
			return false
		}
	}
	if id := strings.TrimSpace(r.Header.Get(activity.HeaderAgentID)); id != "" {
		if existing, ok := o.bindings.ResolveAgent(id); ok && existing != ownerID {
			httpx.ErrorCode(w, http.StatusConflict, "cross_instance_tab",
				fmt.Sprintf("agent %q is bound to instance %q; tab lives on %q", id, existing, ownerID),
				false, nil)
			return false
		}
	}
	return true
}

// proxyToInstanceForRoute handles activity enrichment and the actual proxy
// for a shorthand request that the routing layer resolved to a specific
// instance. tabID may be empty when routing came from an identity binding.
func (o *Orchestrator) proxyToInstanceForRoute(w http.ResponseWriter, r *http.Request, inst *bridge.Instance, tabID string, decision RoutingDecision) {
	if inst == nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("nil instance for routing decision %q", decision))
		return
	}
	activity.EnrichRouteActivity(r)
	update := activity.Update{
		InstanceID:  inst.ID,
		ProfileID:   inst.ProfileID,
		ProfileName: inst.ProfileName,
	}
	if tabID != "" {
		update.TabID = tabID
	}
	activity.EnrichRequest(r, update)

	targetURL, err := o.instancePathURLFromBridge(inst, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, err)
		return
	}
	o.proxyToURL(w, r, targetURL)
}
