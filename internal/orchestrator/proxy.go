package orchestrator

import (
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

	proxyToInstance := func(inst *bridge.Instance) {
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

	// Fast path: Locator cache hit
	if o.instanceMgr != nil {
		if inst, err := o.instanceMgr.FindInstanceByTabID(tabID); err == nil {
			proxyToInstance(inst)
			return
		}
	}

	// Slow path: legacy lookup
	inst, err := o.findRunningInstanceByTabID(tabID)
	if err == nil {
		// Cache for future O(1) lookups
		if o.instanceMgr != nil {
			o.instanceMgr.Locator.Register(tabID, inst.ID)
		}
		proxyToInstance(&inst.Instance)
		return
	}

	// Fallback: when exactly one instance is running, proxy to it even if
	// the dashboard-side tab lookup failed. This lets the child bridge resolve
	// the tab ID directly and avoids false 404s when the dashboard's cached or
	// listed tab IDs momentarily diverge from the child bridge's registry.
	if only := o.singleRunningInstance(); only != nil {
		proxyToInstance(&only.Instance)
		return
	}

	httpx.Error(w, 404, err)
}

// proxyToInstance proxies a request to a specific instance by ID in the path.
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

// proxyToURL proxies an HTTP request to the given target URL.
func (o *Orchestrator) proxyToURL(w http.ResponseWriter, r *http.Request, targetURL *url.URL) {
	iproxy.Forward(w, r, targetURL, iproxy.Options{
		Client: o.client,
		AllowedURL: func(u *url.URL) bool {
			return o.proxyTargetInstance(u) != nil
		},
		RewriteRequest: func(req *http.Request) {
			activity.PropagateHeaders(r.Context(), req)
			if inst := o.proxyTargetInstance(targetURL); inst != nil {
				req.Header.Set(activity.HeaderPTInstance, inst.ID)
				if inst.ProfileID != "" {
					req.Header.Set(activity.HeaderPTProfileID, inst.ProfileID)
				}
				if inst.ProfileName != "" {
					req.Header.Set(activity.HeaderPTProfile, inst.ProfileName)
				}
				o.applyInstanceAuth(req, inst)
			}
		},
	})
}

// findRunningInstanceByTabID finds the instance that owns the given tab.
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
	o.applyInstanceAuth(req, inst)

	// Use WebSocket proxy for proper upgrade
	handlers.ProxyWebSocket(w, req, targetURL.String())
}

func (o *Orchestrator) instancePathURL(inst *InstanceInternal, path, rawQuery string) (*url.URL, error) {
	if inst == nil {
		return nil, fmt.Errorf("instance not found")
	}
	baseURL, err := o.parseHTTPInstanceURL(inst.URL, inst.Port)
	if err != nil {
		return nil, err
	}
	target := &url.URL{
		Scheme:   baseURL.Scheme,
		Host:     baseURL.Host,
		Path:     path,
		RawQuery: rawQuery,
	}
	return target, nil
}

func (o *Orchestrator) instancePathURLFromBridge(inst *bridge.Instance, path, rawQuery string) (*url.URL, error) {
	if inst == nil {
		return nil, fmt.Errorf("instance not found")
	}
	baseURL, err := o.parseHTTPInstanceURL(inst.URL, inst.Port)
	if err != nil {
		return nil, err
	}
	target := &url.URL{
		Scheme:   baseURL.Scheme,
		Host:     baseURL.Host,
		Path:     path,
		RawQuery: rawQuery,
	}
	return target, nil
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
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// classifyLaunchError returns appropriate HTTP status code for launch errors.
func classifyLaunchError(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "cannot contain") || strings.Contains(msg, "cannot be empty") {
		return 400 // Bad Request - validation error
	}
	if strings.Contains(msg, "already") || strings.Contains(msg, "in use") {
		return 409 // Conflict - resource already exists
	}
	return 500 // Internal Server Error
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
