package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
	iproxy "github.com/pinchtab/pinchtab/internal/proxy"
)

// proxyTabRequest is a generic handler that proxies requests to the instance
// that owns the tab specified in the path. Works for any /tabs/{id}/* route.
//
// Uses the instance Manager's Locator for O(1) cached lookups, falling back
// to the legacy O(n×m) bridge query on cache miss.
func (o *Orchestrator) proxyTabRequest(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	// Enrich activity with action/navigate details from the request body
	// before proxying, so the dashboard stream shows meaningful labels.
	activity.EnrichRouteActivity(r)

	inst, err := o.resolveInstanceForTab(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	o.proxyResolvedTab(w, r, inst, tabID)
}

// resolveInstanceForTab decides which running instance owns tabID — the routing
// policy, kept separate from the proxy transport (proxyResolvedTab). It tries
// the O(1) locator cache first, then the bridge-query fallback (registering the
// result on a hit), then — to avoid false 404s when the dashboard's tab list
// momentarily diverges from the child bridge — the single-running-instance
// shortcut. On a total miss it returns the lookup error for the caller to
// surface as 404.
func (o *Orchestrator) resolveInstanceForTab(tabID string) (*bridge.Instance, error) {
	if o.instanceMgr != nil {
		if inst, err := o.instanceMgr.FindInstanceByTabID(tabID); err == nil {
			return inst, nil
		}
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err == nil {
		if o.instanceMgr != nil {
			o.instanceMgr.Locator.Register(tabID, inst.ID)
		}
		return &inst.Instance, nil
	}

	if only := o.singleRunningInstance(); only != nil {
		return &only.Instance, nil
	}

	return nil, err
}

// proxyResolvedTab forwards the request to the resolved instance — the transport
// step, kept separate from the routing policy (resolveInstanceForTab).
func (o *Orchestrator) proxyResolvedTab(w http.ResponseWriter, r *http.Request, inst *bridge.Instance, tabID string) {
	activity.EnrichRequest(r, activity.Update{
		InstanceID:  inst.ID,
		ProfileID:   inst.ProfileID,
		ProfileName: inst.ProfileName,
		TabID:       tabID,
	})
	targetURL, buildErr := o.instancePathURLFromBridge(inst, r.URL.Path, r.URL.RawQuery)
	if buildErr != nil {
		httpx.Error(w, 502, buildErr)
		return
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) proxyToInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()

	if !ok {
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}

	if inst.Status != "running" {
		httpx.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}
	activity.EnrichRequest(r, activity.Update{
		InstanceID:  inst.ID,
		ProfileID:   inst.ProfileID,
		ProfileName: inst.ProfileName,
	})

	targetPath := r.URL.Path
	if len(targetPath) > len("/instances/"+id) {
		targetPath = targetPath[len("/instances/"+id):]
	} else {
		targetPath = ""
	}

	targetURL, err := o.instancePathURL(inst, targetPath, r.URL.RawQuery)
	if err != nil {
		httpx.Error(w, 502, err)
		return
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) proxyToURL(w http.ResponseWriter, r *http.Request, targetURL *url.URL) {
	// Resolve the target instance once; RewriteRequest/OnResponseHeaders always
	// act on targetURL, so they reuse this instead of re-scanning o.instances.
	targetInst := o.proxyTargetInstance(targetURL)
	iproxy.Forward(w, r, targetURL, iproxy.Options{
		Client: o.client,
		AllowedURL: func(u *url.URL) bool {
			// Redirect targets are validated per-hop (instance/SSRF gate). A
			// same-origin u maps to the already-resolved instance; only
			// different-origin redirects need a fresh scan.
			if sameOrigin(u, targetURL) {
				return targetInst != nil
			}
			return o.proxyTargetInstance(u) != nil
		},
		RewriteRequest: func(req *http.Request) {
			activity.PropagateHeaders(r.Context(), req)
			if targetInst != nil {
				req.Header.Set(activity.HeaderPTInstance, targetInst.ID)
				if targetInst.ProfileID != "" {
					req.Header.Set(activity.HeaderPTProfileID, targetInst.ProfileID)
				}
				if targetInst.ProfileName != "" {
					req.Header.Set(activity.HeaderPTProfile, targetInst.ProfileName)
				}
				o.applyInstanceAuth(req, targetInst)
			}
		},
		OnResponseHeaders: func(origReq *http.Request, resp *http.Response) {
			var targetInstanceID string
			if targetInst != nil {
				targetInstanceID = targetInst.ID
			}
			o.handleProxyResponseHeaders(origReq, resp, targetInstanceID)
		},
		OnResponse: enrichActivityFromResponse,
	})
}

// ProxyToTarget proxies a shorthand dashboard request to a managed instance URL,
// preserving orchestrator-side auth injection for the child bridge.
func (o *Orchestrator) ProxyToTarget(w http.ResponseWriter, r *http.Request, target string) {
	targetURL, err := url.Parse(target)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}
	if targetURL.RawQuery == "" {
		targetURL.RawQuery = r.URL.RawQuery
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) findRunningInstanceByTabID(tabID string) (*InstanceInternal, error) {
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		tabs, err := o.fetchTabs(inst)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			if tab.ID == tabID || o.idMgr.TabIDFromCDPTarget(tab.ID) == tabID {
				return inst, nil
			}
		}
	}
	return nil, fmt.Errorf("tab %q not found", tabID)
}

func (o *Orchestrator) handleProxyScreencast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok || inst.Status != "running" {
		httpx.Error(w, 404, fmt.Errorf("instance not found or not running"))
		return
	}
	activity.EnrichRequest(r, activity.Update{
		InstanceID:  inst.ID,
		ProfileID:   inst.ProfileID,
		ProfileName: inst.ProfileName,
	})

	targetURL, err := o.instancePathURL(inst, "/screencast", r.URL.RawQuery)
	if err != nil {
		httpx.Error(w, 502, err)
		return
	}

	req := r.Clone(r.Context())
	req.Header = r.Header.Clone()
	activity.PropagateHeaders(r.Context(), req)
	req.Header.Del("Authorization")
	req.Header.Del("Cookie")
	iproxy.SetProxyWSBackendAuthorization(req.Header, "")
	if token := inst.authToken; token != "" {
		iproxy.SetProxyWSBackendAuthorization(req.Header, "Bearer "+token)
	} else if token := o.childAuthToken; token != "" {
		iproxy.SetProxyWSBackendAuthorization(req.Header, "Bearer "+token)
	}

	iproxy.ProxyWebSocket(w, req, targetURL.String())
}

func (o *Orchestrator) buildInstancePathURL(rawURL, port, path, rawQuery string) (*url.URL, error) {
	baseURL, err := o.parseHTTPInstanceURL(rawURL, port)
	if err != nil {
		return nil, err
	}
	return &url.URL{
		Scheme:   baseURL.Scheme,
		Host:     baseURL.Host,
		Path:     path,
		RawQuery: rawQuery,
	}, nil
}

func (o *Orchestrator) instancePathURL(inst *InstanceInternal, path, rawQuery string) (*url.URL, error) {
	if inst == nil {
		return nil, fmt.Errorf("instance not found")
	}
	return o.buildInstancePathURL(inst.URL, inst.Port, path, rawQuery)
}

func (o *Orchestrator) instancePathURLFromBridge(inst *bridge.Instance, path, rawQuery string) (*url.URL, error) {
	if inst == nil {
		return nil, fmt.Errorf("instance not found")
	}
	return o.buildInstancePathURL(inst.URL, inst.Port, path, rawQuery)
}

func (o *Orchestrator) parseHTTPInstanceURL(rawURL, port string) (*url.URL, error) {
	if rawURL == "" && port != "" {
		rawURL = "http://localhost:" + port
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid instance URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("instance %q is not an HTTP bridge", rawURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid instance URL %q", rawURL)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("instance URL %q must not include a path", rawURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("instance URL %q must not include userinfo", rawURL)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("instance URL %q must not include query or fragment", rawURL)
	}
	return parsed, nil
}

func (o *Orchestrator) proxyTargetInstance(targetURL *url.URL) *InstanceInternal {
	if targetURL == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, inst := range o.instances {
		baseURL, err := o.parseHTTPInstanceURL(inst.URL, inst.Port)
		if err != nil {
			continue
		}
		if sameOrigin(baseURL, targetURL) {
			return inst
		}
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func (o *Orchestrator) applyInstanceAuth(req *http.Request, inst *InstanceInternal) {
	if req == nil || inst == nil {
		return
	}
	token := inst.authToken
	if token == "" {
		token = o.childAuthToken
	}
	if token != "" {
		bearer := "Bearer " + token
		req.Header.Set("Authorization", bearer)
		iproxy.SetProxyWSBackendAuthorization(req.Header, bearer)
	}
	// Mark spawned-child hops as trusted-internal-proxy so the instance
	// honors X-PinchTab-* identity headers we propagate. Attached external
	// bridges have their own auth domain and won't recognize the token,
	// which is the desired behavior.
	if inst.authToken == "" && o.internalToken != "" {
		req.Header.Set(handlers.InternalTokenHeader, o.internalToken)
	}
}

func classifyLaunchError(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "cannot contain") || strings.Contains(msg, "cannot be empty") {
		return 400
	}
	if strings.Contains(msg, "already") || strings.Contains(msg, "in use") {
		return 409
	}
	return 500
}

// enrichActivityFromResponse extracts tabId and url from the bridge JSON
// response and enriches the activity event on the original request so the
// dashboard can link the event to the correct tab.
func enrichActivityFromResponse(origReq *http.Request, body []byte) {
	var resp struct {
		TabID string `json:"tabId"`
		URL   string `json:"url"`
	}
	if json.Unmarshal(body, &resp) != nil {
		return
	}
	update := activity.Update{}
	if resp.TabID != "" {
		update.TabID = resp.TabID
	}
	if resp.URL != "" {
		update.URL = resp.URL
	}
	if update.TabID != "" || update.URL != "" {
		activity.EnrichRequest(origReq, update)
	}
}

func (o *Orchestrator) handleProxyResponseHeaders(origReq *http.Request, resp *http.Response, targetInstanceID string) {
	if o == nil || origReq == nil || resp == nil {
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	if o.instanceMgr != nil {
		if tabID := tabClosePathID(origReq); tabID != "" {
			o.instanceMgr.InvalidateTab(tabID)
		} else if origReq.Method == http.MethodPost && strings.TrimSpace(origReq.URL.Path) == "/close" {
			if tabID := strings.TrimSpace(resp.Header.Get(activity.HeaderPTTabID)); tabID != "" {
				o.instanceMgr.InvalidateTab(tabID)
			}
		}
	}

	// Identity → instance binding writes. Bindings are persisted only after
	// a successful proxy response so failed requests never create or move
	// routing state.
	if o.bindings != nil && targetInstanceID != "" {
		if id := sessionIDForRouting(origReq); id != "" {
			o.bindings.BindSession(id, targetInstanceID)
		}
		if id := strings.TrimSpace(origReq.Header.Get(activity.HeaderAgentID)); id != "" {
			o.bindings.BindAgent(id, targetInstanceID)
		}
	}

	// Tabs cache invalidation. Any successful response that may have
	// changed the instance's tab list (open/close/navigate/history) drops
	// the cached snapshot so the next dashboard read picks up fresh data.
	if o.tabsCache != nil && targetInstanceID != "" && tabsCacheRequestAffectsTabs(origReq, resp) {
		o.tabsCache.Invalidate(targetInstanceID)
	}
}

// tabsCacheRequestAffectsTabs reports whether a successful response should
// invalidate the per-instance tabs cache. Errs on the side of invalidating
// rather than serving stale data — the cache is a perf optimization, not
// a correctness guarantee.
func tabsCacheRequestAffectsTabs(req *http.Request, resp *http.Response) bool {
	if req == nil {
		return false
	}
	// X-PinchTab-Tab-Id is a strong signal something changed; invalidate
	// regardless of the route the request hit.
	if resp != nil {
		if strings.TrimSpace(resp.Header.Get(activity.HeaderPTTabID)) != "" {
			return true
		}
	}
	if req.Method != http.MethodPost {
		return false
	}
	path := strings.TrimSpace(req.URL.Path)
	switch path {
	case "/tab", "/close", "/navigate", "/reload", "/back", "/forward":
		return true
	}
	if subpath := instanceRouteSubpath(path); subpath != "" {
		switch subpath {
		case "/tab", "/close", "/navigate", "/reload", "/back", "/forward":
			return true
		}
		if strings.HasPrefix(subpath, "/tabs/") {
			switch {
			case strings.HasSuffix(subpath, "/close"),
				strings.HasSuffix(subpath, "/navigate"),
				strings.HasSuffix(subpath, "/reload"),
				strings.HasSuffix(subpath, "/back"),
				strings.HasSuffix(subpath, "/forward"):
				return true
			}
		}
	}
	if strings.HasPrefix(path, "/tabs/") {
		switch {
		case strings.HasSuffix(path, "/close"),
			strings.HasSuffix(path, "/navigate"),
			strings.HasSuffix(path, "/reload"),
			strings.HasSuffix(path, "/back"),
			strings.HasSuffix(path, "/forward"):
			return true
		}
	}
	return false
}

func instanceRouteSubpath(path string) string {
	if !strings.HasPrefix(path, "/instances/") {
		return ""
	}
	rest := strings.TrimPrefix(path, "/instances/")
	_, subpath, ok := strings.Cut(rest, "/")
	if !ok || subpath == "" {
		return ""
	}
	return "/" + subpath
}

func tabClosePathID(r *http.Request) string {
	if r == nil || r.Method != http.MethodPost {
		return ""
	}
	if !strings.HasPrefix(r.URL.Path, "/tabs/") || !strings.HasSuffix(r.URL.Path, "/close") {
		return ""
	}
	return strings.TrimSpace(r.PathValue("id"))
}

func (o *Orchestrator) singleRunningInstance() *InstanceInternal {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var only *InstanceInternal
	for _, inst := range o.instances {
		if inst.Status != "running" || !instanceIsActive(inst) {
			continue
		}
		if only != nil {
			return nil
		}
		only = inst
	}
	return only
}
