package orchestrator

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (o *Orchestrator) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o.mu.RLock()
	inst, ok := o.instances[id]
	if !ok {
		o.mu.RUnlock()
		web.Error(w, 404, fmt.Errorf("instance %q not found", id))
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

	web.JSON(w, 200, copyInst)
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
			web.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	headless := req.Mode != "headed"

	var name string
	if req.ProfileId != "" {
		profs, err := o.profiles.List()
		if err != nil {
			web.Error(w, 500, fmt.Errorf("failed to list profiles: %w", err))
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
			web.Error(w, 400, fmt.Errorf("profile %q not found", req.ProfileId))
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
		web.Error(w, statusCode, err)
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

func (o *Orchestrator) handleStartByInstanceID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	if !ok {
		o.mu.RUnlock()
		web.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}
	active := instanceIsActive(inst)
	port := inst.Port
	profileName := inst.ProfileName
	headless := inst.Headless
	o.mu.RUnlock()

	if active {
		targetURL := &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("localhost", port),
			Path:   "/ensure-chrome",
		}
		o.proxyToURL(w, r, targetURL)
		return
	}

	started, err := o.Launch(profileName, port, headless, nil)
	if err != nil {
		statusCode := classifyLaunchError(err)
		web.Error(w, statusCode, err)
		return
	}
	web.JSON(w, 201, started)
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

func (o *Orchestrator) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID      string   `json:"profileId,omitempty"`
		Mode           string   `json:"mode,omitempty"`
		Port           string   `json:"port,omitempty"`
		ExtensionPaths []string `json:"extensionPaths,omitempty"`
	}

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	var profileName string
	var err error

	if req.ProfileID != "" {
		profileName, err = o.resolveProfileName(req.ProfileID)
		if err != nil {
			web.Error(w, 404, fmt.Errorf("profile %q not found", req.ProfileID))
			return
		}
	} else {
		profileName = fmt.Sprintf("instance-%d", time.Now().UnixNano())
	}

	headless := req.Mode != "headed"

	inst, err := o.Launch(profileName, req.Port, headless, req.ExtensionPaths)
	if err != nil {
		statusCode := classifyLaunchError(err)
		web.Error(w, statusCode, err)
		return
	}

	web.JSON(w, 201, inst)
}

func (o *Orchestrator) handleInstanceTabs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()

	if !ok {
		web.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}

	if inst.Status != "running" || !instanceIsActive(inst) {
		web.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	tabs, err := o.fetchTabs(inst.URL)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("failed to fetch tabs for instance %q: %w", id, err))
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

	web.JSON(w, 200, result)
}

func (o *Orchestrator) handleAttachInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CdpURL string `json:"cdpUrl"`
		Name   string `json:"name,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("invalid JSON"))
		return
	}

	if req.CdpURL == "" {
		web.Error(w, 400, fmt.Errorf("cdpUrl is required"))
		return
	}

	// Validate attach is enabled and URL is allowed
	if err := o.validateAttachURL(req.CdpURL); err != nil {
		web.Error(w, 403, err)
		return
	}

	// Generate name if not provided
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("attached-%d", time.Now().UnixNano())
	}

	inst, err := o.Attach(name, req.CdpURL)
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	web.JSON(w, 201, inst)
}

// validateAttachURL checks if attach is enabled and the CDP URL is allowed.
func (o *Orchestrator) validateAttachURL(cdpURL string) error {
	if o.runtimeCfg == nil {
		return fmt.Errorf("attach not configured")
	}

	if !o.runtimeCfg.AttachEnabled {
		return fmt.Errorf("attach is disabled")
	}

	parsed, err := url.Parse(cdpURL)
	if err != nil {
		return fmt.Errorf("invalid cdpUrl: %w", err)
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
