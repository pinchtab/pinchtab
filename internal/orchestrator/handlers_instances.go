package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (o *Orchestrator) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o.mu.RLock()
	inst, ok := o.instances[id]
	if !ok {
		o.mu.RUnlock()
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}

	copyInst := inst.Instance
	active := instanceIsActive(inst)
	o.mu.RUnlock()

	if active && copyInst.Status == "stopped" {
		copyInst.Status = "running"
	}
	if !active &&
		(copyInst.Status == "starting" || copyInst.Status == "running" || copyInst.Status == "stopping") {
		copyInst.Status = "stopped"
	}

	httpx.JSON(w, 200, copyInst)
}

func (o *Orchestrator) handleLaunchByName(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileId      string   `json:"profileId,omitempty"`
		Name           string   `json:"name,omitempty"`
		Mode           string   `json:"mode"`
		Port           string   `json:"port,omitempty"`
		ExtensionPaths []string `json:"extensionPaths,omitempty"`
	}

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	headless := req.Mode != "headed"

	var name string
	if req.ProfileId != "" {
		profs, err := o.profiles.List()
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("failed to list profiles: %w", err))
			return
		}
		found := false
		for _, p := range profs {
			if p.ID == req.ProfileId {
				name = p.Name
				found = true
				break
			}
		}
		if !found {
			httpx.Error(w, 400, fmt.Errorf("profile %q not found", req.ProfileId))
			return
		}
	} else if req.Name != "" {
		name = req.Name
	} else {
		name = fmt.Sprintf("instance-%d", time.Now().UnixNano())
	}

	inst, err := o.Launch(name, req.Port, headless, req.ExtensionPaths)
	if err != nil {
		statusCode := classifyLaunchError(err)
		httpx.Error(w, statusCode, err)
		return
	}
	authn.AuditLog(r, "instance.launched", "profileName", name, "instanceId", inst.ID)
	httpx.JSON(w, 201, inst)
}

func (o *Orchestrator) handleStopByInstanceID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := o.Stop(id); err != nil {
		httpx.Error(w, 404, err)
		return
	}
	authn.AuditLog(r, "instance.stopped", "instanceId", id)
	httpx.JSON(w, 200, map[string]string{"status": "stopped", "id": id})
}

func (o *Orchestrator) handleStartByInstanceID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	if !ok {
		o.mu.RUnlock()
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}
	active := instanceIsActive(inst)
	port := inst.Port
	profileName := inst.ProfileName
	headless := inst.Headless
	o.mu.RUnlock()

	if inst.Attached && inst.AttachType != "bridge" {
		httpx.Error(w, 409, fmt.Errorf("attached instance %q cannot be started by the orchestrator", id))
		return
	}

	if active {
		targetURL, targetErr := o.instancePathURL(inst, "/ensure-chrome", "")
		if targetErr != nil {
			httpx.Error(w, 502, targetErr)
			return
		}
		o.proxyToURL(w, r, targetURL)
		return
	}

	if inst.Attached {
		httpx.Error(w, 409, fmt.Errorf("attached instance %q cannot be started by the orchestrator", id))
		return
	}

	started, err := o.Launch(profileName, port, headless, nil)
	if err != nil {
		statusCode := classifyLaunchError(err)
		httpx.Error(w, statusCode, err)
		return
	}
	authn.AuditLog(r, "instance.started", "instanceId", started.ID, "profileName", profileName)
	httpx.JSON(w, 201, started)
}

func (o *Orchestrator) handleLogsByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logs, err := o.Logs(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(logs))
}

func (o *Orchestrator) handleLogsStreamByID(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		http.Error(w, "streaming deadline unsupported", http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	initial, err := o.Logs(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	writeLogs := func(logs string) bool {
		data, err := json.Marshal(map[string]string{"logs": logs})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !writeLogs(initial) {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	last := initial
	for {
		select {
		case <-ticker.C:
			current, err := o.Logs(id)
			if err != nil {
				return
			}
			if current != last {
				last = current
				if !writeLogs(current) {
					return
				}
				continue
			}
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (o *Orchestrator) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID      string   `json:"profileId,omitempty"`
		Mode           string   `json:"mode,omitempty"`
		Port           string   `json:"port,omitempty"`
		ExtensionPaths []string `json:"extensionPaths,omitempty"`
	}

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	var profileName string
	var err error

	if req.ProfileID != "" {
		profileName, err = o.resolveProfileName(req.ProfileID)
		if err != nil {
			httpx.Error(w, 404, fmt.Errorf("profile %q not found", req.ProfileID))
			return
		}
	} else {
		profileName = fmt.Sprintf("instance-%d", time.Now().UnixNano())
	}

	headless := req.Mode != "headed"

	inst, err := o.Launch(profileName, req.Port, headless, req.ExtensionPaths)
	if err != nil {
		statusCode := classifyLaunchError(err)
		httpx.Error(w, statusCode, err)
		return
	}

	authn.AuditLog(r, "instance.started", "instanceId", inst.ID, "profileName", profileName)
	httpx.JSON(w, 201, inst)
}

func (o *Orchestrator) handleInstanceTabs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()

	if !ok {
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}

	if inst.Status != "running" || !instanceIsActive(inst) {
		httpx.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	tabs, err := o.fetchTabs(inst)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("failed to fetch tabs for instance %q: %w", id, err))
		return
	}

	result := make([]map[string]any, 0, len(tabs))
	for _, tab := range tabs {
		result = append(result, map[string]any{
			"id":         tab.ID,
			"instanceId": inst.ID,
			"url":        tab.URL,
			"title":      tab.Title,
		})
	}

	httpx.JSON(w, 200, result)
}

func (o *Orchestrator) handleAttachInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CdpURL string `json:"cdpUrl"`
		Name   string `json:"name,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("invalid JSON"))
		return
	}

	if req.CdpURL == "" {
		httpx.Error(w, 400, fmt.Errorf("cdpUrl is required"))
		return
	}

	// Validate attach is enabled and URL is allowed
	if err := o.validateAttachURL(req.CdpURL); err != nil {
		httpx.Error(w, 403, err)
		return
	}

	// Generate name if not provided
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("attached-%d", time.Now().UnixNano())
	}

	inst, err := o.Attach(name, req.CdpURL)
	if err != nil {
		httpx.Error(w, classifyLaunchError(err), err)
		return
	}

	authn.AuditLog(r, "instance.attached", "instanceId", inst.ID, "instanceName", inst.ProfileName, "attachType", "cdp")
	httpx.JSON(w, 201, inst)
}

func (o *Orchestrator) handleAttachBridge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"baseUrl"`
		Name    string `json:"name,omitempty"`
		Token   string `json:"token,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("invalid JSON"))
		return
	}
	if req.BaseURL == "" {
		httpx.Error(w, 400, fmt.Errorf("baseUrl is required"))
		return
	}
	if err := o.validateAttachURL(req.BaseURL); err != nil {
		httpx.Error(w, 403, err)
		return
	}
	if err := o.probeAttachBridge(req.BaseURL, req.Token); err != nil {
		httpx.Error(w, 502, err)
		return
	}

	name := req.Name
	if name == "" {
		name = fmt.Sprintf("bridge-%d", time.Now().UnixNano())
	}

	inst, err := o.AttachBridge(name, req.BaseURL, req.Token)
	if err != nil {
		httpx.Error(w, classifyLaunchError(err), err)
		return
	}
	authn.AuditLog(r, "instance.attached", "instanceId", inst.ID, "instanceName", inst.ProfileName, "attachType", "bridge")
	httpx.JSON(w, 201, inst)
}

// probeAttachBridge checks that a remote bridge is reachable.
// The baseURL MUST have been validated by validateAttachURL before calling this.
func (o *Orchestrator) probeAttachBridge(baseURL, token string) error {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return fmt.Errorf("invalid bridge baseUrl: %w", err)
	}

	// Re-validate scheme inside this function so the safety check is visible
	// to static analysis (CodeQL SSRF). validateAttachURL already enforces
	// the allowlist, but CodeQL can't trace across function boundaries.
	switch parsed.Scheme {
	case "http", "https":
		// allowed
	default:
		return fmt.Errorf("unsupported bridge scheme %q", parsed.Scheme)
	}

	healthURL := parsed.Scheme + "://" + parsed.Host + "/health"

	req, err := http.NewRequest(http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("build bridge health request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("bridge health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// validateAttachURL checks if attach is enabled and the external URL is allowed.
func (o *Orchestrator) validateAttachURL(rawURL string) error {
	if o.runtimeCfg == nil {
		return fmt.Errorf("attach not configured")
	}

	if !o.runtimeCfg.AttachEnabled {
		return fmt.Errorf("attach is disabled")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid attach URL: %w", err)
	}

	// Validate scheme
	schemeAllowed := false
	for _, allowed := range o.runtimeCfg.AttachAllowSchemes {
		if parsed.Scheme == allowed {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed {
		return fmt.Errorf("scheme %q not allowed (allowed: %v)", parsed.Scheme, o.runtimeCfg.AttachAllowSchemes)
	}

	if (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("bridge baseUrl must not include a path")
	}

	// Validate host
	host := parsed.Hostname()
	hostAllowed := false
	for _, allowed := range o.runtimeCfg.AttachAllowHosts {
		if host == allowed {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return fmt.Errorf("host %q not allowed (allowed: %v)", host, o.runtimeCfg.AttachAllowHosts)
	}

	return nil
}
