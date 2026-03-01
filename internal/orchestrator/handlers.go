package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	// Core routes
	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("GET /instances/tabs", o.handleAllTabs)

	// Phase 3: Tab Management (new API)
	// Note: GET /tabs and POST /tab handlers are in dashboard.go
	// These orchestrator handlers are used when no instances are running or for aggregation
	mux.HandleFunc("POST /tabs/open", o.handleTabOpen)
	// Commented out to avoid conflict with dashboard's GET /tabs
	// mux.HandleFunc("GET /tabs", o.handleTabList)
	mux.HandleFunc("GET /tabs/{id}", o.handleTabGet)
	// mux.HandleFunc("POST /tabs/{id}/close", o.handleTabClose)  // Handled by dashboard's POST /tab

	// Profile lifecycle by ID (canonical)
	mux.HandleFunc("POST /profiles/{id}/start", o.handleStartByID)
	mux.HandleFunc("POST /profiles/{id}/stop", o.handleStopByID)
	mux.HandleFunc("GET /profiles/{id}/instance", o.handleProfileInstance)

	// Short aliases for agents
	mux.HandleFunc("POST /start/{id}", o.handleStartByID)
	mux.HandleFunc("POST /stop/{id}", o.handleStopByID)

	// Phase 2: Instance Management (new API)
	mux.HandleFunc("POST /instances/start", o.handleStartInstance)

	// Dashboard / backward compat
	mux.HandleFunc("POST /instances/launch", o.handleLaunchByName)
	mux.HandleFunc("POST /instances/{id}/stop", o.handleStopByInstanceID)
	mux.HandleFunc("GET /instances/{id}/logs", o.handleLogsByID)
	mux.HandleFunc("GET /instances/{id}/proxy/screencast", o.handleProxyScreencast)

	// Instance proxy routes - forward to specific instance port
	// Browser operations
	mux.HandleFunc("POST /instances/{id}/navigate", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/snapshot", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/action", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/actions", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/screenshot", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/pdf", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/text", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/evaluate", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/tabs", o.proxyToInstance)

	// Chrome management
	mux.HandleFunc("POST /instances/{id}/ensure-chrome", o.proxyToInstance)

	// Tab management
	mux.HandleFunc("POST /instances/{id}/tab", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/tab/lock", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/tab/unlock", o.proxyToInstance)

	// Other operations
	mux.HandleFunc("GET /instances/{id}/cookies", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/cookies", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/download", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/upload", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/screencast", o.proxyToInstance)
}

func (o *Orchestrator) handleList(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, o.List())
}

// resolveProfileID resolves a path value to a profile name.
// Accepts both a 12-char hex profile ID or a profile name directly.
func (o *Orchestrator) resolveProfileName(idOrName string) (string, error) {
	if o.profiles == nil {
		return "", fmt.Errorf("profile manager not configured")
	}
	// Try as ID first
	if name, err := o.profiles.FindByID(idOrName); err == nil {
		return name, nil
	}
	// Try as name (for backward compat routes like /profiles/{name}/stop)
	if o.profiles.Exists(idOrName) {
		return idOrName, nil
	}
	return "", fmt.Errorf("profile %q not found", idOrName)
}

func (o *Orchestrator) handleStartByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := o.resolveProfileName(id)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	var req struct {
		Port     string `json:"port,omitempty"`
		Headless bool   `json:"headless"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	// Port is optional - if not provided, Launch() will auto-allocate

	inst, err := o.Launch(name, req.Port, req.Headless)
	if err != nil {
		web.Error(w, 409, err)
		return
	}
	web.JSON(w, 201, inst)
}

func (o *Orchestrator) handleStopByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := o.resolveProfileName(id)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	if err := o.StopProfile(name); err != nil {
		web.Error(w, 404, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "stopped", "id": id, "name": name})
}

func (o *Orchestrator) handleLaunchByName(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"` // "headed" or "headless" (default: headless)
		Port string `json:"port,omitempty"`
	}

	// Decode body if present (empty body is allowed)
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	// Default: headless=true unless mode="headed"
	headless := true
	if req.Mode == "headed" {
		headless = false
	}

	// Generate unique instance name (internal use only)
	name := fmt.Sprintf("instance-%d", time.Now().UnixNano())

	// Port is optional - if not provided, Launch() will auto-allocate
	inst, err := o.Launch(name, req.Port, headless)
	if err != nil {
		web.Error(w, 409, err)
		return
	}
	web.JSON(w, 201, inst)
}

func (o *Orchestrator) handleStopByInstanceID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := o.Stop(id); err != nil {
		web.Error(w, 404, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "stopped", "id": id})
}

func (o *Orchestrator) handleLogsByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logs, err := o.Logs(id)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(logs))
}

// handleStartInstance: Phase 2 API endpoint for starting instances
// POST /instances/start {profileId?, mode?, port?}
// All parameters are optional
func (o *Orchestrator) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID string `json:"profileId,omitempty"` // Optional: Profile ID or name
		Mode      string `json:"mode,omitempty"`      // Optional: "headed" or "headless"
		Port      string `json:"port,omitempty"`      // Optional: port number
	}

	// Decode body if present (empty body is allowed)
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	// Resolve profileId to profile name
	// If profileId not provided, auto-generate instance with default profile or first available
	var profileName string
	var err error

	if req.ProfileID != "" {
		// User provided profileId (or name) - resolve it
		profileName, err = o.resolveProfileName(req.ProfileID)
		if err != nil {
			web.Error(w, 404, fmt.Errorf("profile %q not found", req.ProfileID))
			return
		}
	} else {
		// No profileId provided - use auto-generated name (creates temporary profile)
		profileName = fmt.Sprintf("instance-%d", time.Now().UnixNano())
	}

	// Parse mode: "headed" or "headless" (default: headless)
	headless := true
	if req.Mode == "headed" {
		headless = false
	}

	// Port is optional - Launch() will auto-allocate if not provided
	inst, err := o.Launch(profileName, req.Port, headless)
	if err != nil {
		web.Error(w, 409, err)
		return
	}

	web.JSON(w, 201, inst)
}

func (o *Orchestrator) handleAllTabs(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, o.AllTabs())
}

func (o *Orchestrator) handleProfileInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := o.resolveProfileName(id)
	if err != nil {
		web.JSON(w, 200, map[string]any{
			"name":    id,
			"running": false,
			"status":  "stopped",
			"port":    "",
		})
		return
	}

	instances := o.List()
	for _, inst := range instances {
		if inst.ProfileName == name && (inst.Status == "running" || inst.Status == "starting") {
			web.JSON(w, 200, map[string]any{
				"name":    name,
				"running": inst.Status == "running",
				"status":  inst.Status,
				"port":    inst.Port,
				"id":      inst.ID,
			})
			return
		}
	}
	web.JSON(w, 200, map[string]any{
		"name":    name,
		"running": false,
		"status":  "stopped",
		"port":    "",
	})
}

func (o *Orchestrator) handleProxyScreencast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tabID := r.URL.Query().Get("tabId")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok || inst.Status != "running" {
		web.Error(w, 404, fmt.Errorf("instance not found or not running"))
		return
	}

	targetURL := fmt.Sprintf("ws://localhost:%s/screencast?tabId=%s", inst.Port, tabID)
	web.JSON(w, 200, map[string]string{"wsUrl": targetURL})
}

// proxyToInstance forwards requests to a specific instance port
// This allows clients to call /instances/{id}/navigate instead of knowing the instance port
func (o *Orchestrator) proxyToInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()

	if !ok {
		web.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}

	if inst.Status != "running" {
		web.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	// Build target URL by replacing /instances/{id} with the instance port path
	// Request: POST /instances/work-9868/navigate?url=...
	// Target:  POST http://localhost:9868/navigate?url=...
	targetPath := r.URL.Path
	// Remove /instances/{id} prefix (19 + len(id) characters)
	if len(targetPath) > len("/instances/"+id) {
		targetPath = targetPath[len("/instances/"+id):]
	} else {
		targetPath = ""
	}

	targetURL := fmt.Sprintf("http://localhost:%s%s", inst.Port, targetPath)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("failed to create proxy request: %w", err))
		return
	}

	// Copy headers from original request (except Host and hop-by-hop headers)
	for key, values := range r.Header {
		switch key {
		case "Host", "Connection", "Keep-Alive", "Proxy-Authenticate",
			"Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade":
			// Skip hop-by-hop headers
		default:
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	// Use orchestrator's HTTP client
	resp, err := o.client.Do(proxyReq)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("failed to proxy to instance: %w", err))
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy response status and body
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(readResponseBody(resp))
}

// readResponseBody reads the full response body
func readResponseBody(resp *http.Response) []byte {
	if resp.Body == nil {
		return []byte{}
	}
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return body
}

// Phase 3: Tab Management Handlers

// handleTabOpen: POST /tabs/open {instanceId, url?}
func (o *Orchestrator) handleTabOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstanceID string `json:"instanceId"` // Required: instance ID
		URL        string `json:"url,omitempty"`     // Optional: URL to open
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("invalid JSON"))
		return
	}

	if req.InstanceID == "" {
		web.Error(w, 400, fmt.Errorf("instanceId is required"))
		return
	}

	// Check if instance exists
	o.mu.RLock()
	_, ok := o.instances[req.InstanceID]
	o.mu.RUnlock()

	if !ok {
		web.Error(w, 404, fmt.Errorf("instance %q not found", req.InstanceID))
		return
	}

	// Generate tab ID
	tabID := fmt.Sprintf("tab_%d", time.Now().UnixNano()%1000000)

	// Build response
	tab := map[string]any{
		"id":         tabID,
		"instanceId": req.InstanceID,
		"url":        req.URL,
		"title":      "",
		"status":     "ready",
		"createdAt":  time.Now().UTC(),
	}

	web.JSON(w, 201, tab)
}

// handleTabList: GET /tabs?instanceId=...
func (o *Orchestrator) handleTabList(w http.ResponseWriter, r *http.Request) {
	instanceID := r.URL.Query().Get("instanceId")

	// Get all instances and their tabs via proxying
	// For now, return a simple response indicating tabs can be queried
	tabs := []map[string]any{}

	if instanceID != "" {
		// Filter tabs for specific instance
		o.mu.RLock()
		instInternal, ok := o.instances[instanceID]
		o.mu.RUnlock()

		if !ok {
			web.Error(w, 404, fmt.Errorf("instance %q not found", instanceID))
			return
		}

		// Query instance for its tabs
		// This would proxy to the instance's /tabs endpoint
		tabResp, err := o.client.Get(fmt.Sprintf("http://localhost:%s/tabs", instInternal.Port))
		if err != nil {
			web.Error(w, 500, fmt.Errorf("failed to get tabs from instance: %w", err))
			return
		}
		defer tabResp.Body.Close()

		if tabResp.StatusCode == 200 {
			var instanceTabs []map[string]any
			json.NewDecoder(tabResp.Body).Decode(&instanceTabs)
			// Add instanceId to each tab
			for _, tab := range instanceTabs {
				tab["instanceId"] = instanceID
				tabs = append(tabs, tab)
			}
		}
	} else {
		// List all tabs across all instances
		o.mu.RLock()
		instances := make([]*InstanceInternal, 0, len(o.instances))
		for _, inst := range o.instances {
			instances = append(instances, inst)
		}
		o.mu.RUnlock()

		for _, inst := range instances {
			if inst.Status != "running" {
				continue
			}
			// Query instance for its tabs
			tabResp, err := o.client.Get(fmt.Sprintf("http://localhost:%s/tabs", inst.Port))
			if err != nil {
				continue
			}
			defer tabResp.Body.Close()

			if tabResp.StatusCode == 200 {
				var instanceTabs []map[string]any
				json.NewDecoder(tabResp.Body).Decode(&instanceTabs)
				// Add instanceId to each tab
				for _, tab := range instanceTabs {
					tab["instanceId"] = inst.ID
					tabs = append(tabs, tab)
				}
			}
		}
	}

	web.JSON(w, 200, tabs)
}

// handleTabGet: GET /tabs/{id}
func (o *Orchestrator) handleTabGet(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")

	// Find which instance has this tab
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		instances = append(instances, inst)
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		if inst.Status != "running" {
			continue
		}
		// Query instance for this specific tab
		tabResp, err := o.client.Get(fmt.Sprintf("http://localhost:%s/tabs/%s", inst.Port, tabID))
		if err != nil {
			continue
		}
		defer tabResp.Body.Close()

		if tabResp.StatusCode == 200 {
			var tab map[string]any
			json.NewDecoder(tabResp.Body).Decode(&tab)
			tab["instanceId"] = inst.ID
			web.JSON(w, 200, tab)
			return
		}
	}

	web.Error(w, 404, fmt.Errorf("tab %q not found", tabID))
}

// handleTabClose: POST /tabs/{id}/close
func (o *Orchestrator) handleTabClose(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")

	// Find which instance has this tab and close it
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		instances = append(instances, inst)
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		if inst.Status != "running" {
			continue
		}
		// Try to close tab in this instance
		closeResp, err := o.client.Post(
			fmt.Sprintf("http://localhost:%s/tabs/%s/close", inst.Port, tabID),
			"application/json",
			nil,
		)
		if err != nil {
			continue
		}
		defer closeResp.Body.Close()

		if closeResp.StatusCode == 200 || closeResp.StatusCode == 204 {
			web.JSON(w, 200, map[string]string{"id": tabID, "status": "closed"})
			return
		}
	}

	web.Error(w, 404, fmt.Errorf("tab %q not found", tabID))
}
