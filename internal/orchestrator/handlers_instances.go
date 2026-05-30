package orchestrator

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type startInstanceRequest struct {
	ProfileID       string                 `json:"profileId,omitempty"`
	Mode            string                 `json:"mode,omitempty"`
	Port            string                 `json:"port,omitempty"`
	ExtensionPaths  []string               `json:"extensionPaths,omitempty"`
	SecurityPolicy  *bridge.SecurityPolicy `json:"securityPolicy,omitempty"`
	Browser         string                 `json:"browser,omitempty"`
	FallbackTargets []string               `json:"fallbackTargets,omitempty"`
}

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
		startInstanceRequest
		Name string `json:"name,omitempty"`
	}

	if r.ContentLength > 0 {
		if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
			httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
			return
		}
	}

	if req.Name != "" {
		httpx.Error(w, 400, fmt.Errorf("name is not supported on /instances/launch; create the profile first via /profiles and then use profileId"))
		return
	}

	o.startInstanceWithRequest(w, r, req.startInstanceRequest, "instance.launched")
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

func (o *Orchestrator) handleRestartByInstanceID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok {
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}
	if !instanceIsActive(inst) || inst.Status != "running" {
		httpx.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	targetURL, err := o.instancePathURL(inst, "/browser/restart", "")
	if err != nil {
		httpx.Error(w, 502, err)
		return
	}
	o.proxyToURL(w, r, targetURL)
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

	started, err := o.LaunchWithOptions(profileName, port, headless, LaunchOptions{
		SecurityPolicy: inst.requestedSecurityPolicy,
	})
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
		httpx.Problem(w, http.StatusInternalServerError, "streaming_not_supported", "streaming not supported", false, nil)
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		httpx.Problem(w, http.StatusInternalServerError, "streaming_deadline_unsupported", "streaming deadline unsupported", false, nil)
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
	var req startInstanceRequest

	if r.ContentLength > 0 {
		if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
			httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
			return
		}
	}

	o.startInstanceWithRequest(w, r, req, "instance.started")
}

func (o *Orchestrator) startInstanceWithRequest(w http.ResponseWriter, r *http.Request, req startInstanceRequest, auditEvent string) {
	if len(req.ExtensionPaths) > 0 {
		httpx.Error(w, 400, fmt.Errorf("extensionPaths are not supported on instance start requests; configure browser.extensionPaths on the server instead"))
		return
	}
	if err := validateStartInstanceSecurityPolicy(req.SecurityPolicy); err != nil {
		httpx.Error(w, 400, err)
		return
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
		var rnd [4]byte
		_, _ = rand.Read(rnd[:])
		profileName = fmt.Sprintf("instance-%d-%x", time.Now().UnixNano(), rnd)
	}

	headless := req.Mode != "headed"

	opts := LaunchOptions{
		ExtensionPaths: req.ExtensionPaths,
		SecurityPolicy: req.SecurityPolicy,
		Browser:        req.Browser,
	}

	inst, err := o.LaunchWithTargetSelection(profileName, req.Port, headless, req.Browser, req.FallbackTargets, opts)
	if err != nil {
		writeLaunchError(w, err)
		return
	}

	authn.AuditLog(r, auditEvent, "instanceId", inst.ID, "profileName", profileName)
	httpx.JSON(w, 201, inst)
}

// writeLaunchError maps launch / fallback errors to 400 (unknown target), 502 (exhaustion), or the legacy classifier.
func writeLaunchError(w http.ResponseWriter, err error) {
	var unknown *UnknownBrowserError
	if errors.As(err, &unknown) {
		httpx.Error(w, http.StatusBadRequest, err)
		return
	}
	var exhausted *FallbackExhaustedError
	if errors.As(err, &exhausted) {
		attempts := make([]map[string]string, 0, len(exhausted.Attempts))
		for _, a := range exhausted.Attempts {
			attempts = append(attempts, map[string]string{
				"target": a.Target,
				"reason": string(a.Reason),
			})
		}
		httpx.ErrorCode(w, http.StatusBadGateway, "browser_target_unavailable", err.Error(), true, map[string]any{
			"attempts": attempts,
		})
		return
	}
	httpx.Error(w, classifyLaunchError(err), err)
}

func validateStartInstanceSecurityPolicy(policy *bridge.SecurityPolicy) error {
	if policy == nil {
		return nil
	}
	errs := config.ValidateFileConfig(&config.FileConfig{
		Security: config.SecurityConfig{
			AllowedDomains: append([]string(nil), policy.AllowedDomains...),
			IDPI: config.IDPIConfig{
				Enabled: true,
			},
		},
	})
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid securityPolicy.allowedDomains: %w", errs[0])
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

	fresh := r.URL.Query().Get("fresh") == "1"
	tabs, err := o.instanceTabsCached(inst, fresh)
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
		CdpURL   string `json:"cdpUrl"`
		Name     string `json:"name,omitempty"`
		Provider string `json:"provider,omitempty"`
		Browser  string `json:"browser,omitempty"`
	}

	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
		return
	}

	if req.CdpURL == "" {
		httpx.Error(w, 400, fmt.Errorf("cdpUrl is required"))
		return
	}

	// Validate browser/provider conflict: if both are set they must agree.
	if req.Browser != "" && req.Provider != "" {
		normBrowser := config.NormalizeBrowser(req.Browser)
		normProvider := config.NormalizeBrowser(req.Provider)
		if normBrowser != normProvider {
			httpx.Error(w, 400, fmt.Errorf("browser provider %q conflicts with browserTarget %q provider %q", req.Provider, req.Browser, normBrowser))
			return
		}
	}

	attachBrowser := req.Browser
	if attachBrowser == "" && req.Provider != "" {
		attachBrowser = req.Provider
	}

	// When targets are configured, validate the requested browser maps to at
	// least one target before attempting the attach.
	if attachBrowser != "" && o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		matches := config.TargetsForBrowser(o.runtimeCfg, attachBrowser)
		if len(matches) == 0 {
			httpx.Error(w, 400, fmt.Errorf("no browser target configured for browser %q", attachBrowser))
			return
		}
	}

	attachOpts, err := o.resolveAttachOptions(AttachOptions{
		Browser: attachBrowser,
	}, config.BrowserChrome)
	if err != nil {
		httpx.Error(w, 400, err)
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

	inst, err := o.AttachWithOptions(name, req.CdpURL, attachOpts)
	if err != nil {
		httpx.Error(w, classifyLaunchError(err), err)
		return
	}

	authn.AuditLog(r, "instance.attached", "instanceId", inst.ID, "instanceName", inst.ProfileName, "attachType", "cdp-bridge")
	httpx.JSON(w, 201, inst)
}

func (o *Orchestrator) handleAttachBridge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"baseUrl"`
		Name    string `json:"name,omitempty"`
		Token   string `json:"token,omitempty"`
		Browser string `json:"browser,omitempty"`
	}

	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
		return
	}
	if req.BaseURL == "" {
		httpx.Error(w, 400, fmt.Errorf("baseUrl is required"))
		return
	}

	attachOpts, err := o.resolveAttachOptions(AttachOptions{Browser: req.Browser}, "")
	if err != nil {
		httpx.Error(w, 400, err)
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

	inst, created, err := o.AttachBridgeWithOptions(name, req.BaseURL, req.Token, attachOpts)
	if err != nil {
		httpx.Error(w, classifyLaunchError(err), err)
		return
	}
	if created {
		authn.AuditLog(r, "instance.attached", "instanceId", inst.ID, "instanceName", inst.ProfileName, "attachType", "bridge")
		httpx.JSON(w, 201, inst)
	} else {
		authn.AuditLog(r, "instance.reattached", "instanceId", inst.ID, "instanceName", inst.ProfileName, "attachType", "bridge")
		httpx.JSON(w, 200, inst)
	}
}

// probeAttachBridge checks that a remote bridge is reachable.
// The baseURL MUST have been validated by validateAttachURL before calling this.
func (o *Orchestrator) probeAttachBridge(baseURL, token string) error {
	targetBaseURL, err := o.validatedHealthProbeBaseURL(strings.TrimRight(baseURL, "/"), "", healthProbePolicyAttachAllowlist)
	if err != nil {
		return fmt.Errorf("invalid bridge baseUrl: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, healthProbeURL(targetBaseURL), nil)
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

	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		// CDP attach allows /json/version; attach-bridge requires bare origin.
		if parsed.Path != "" && parsed.Path != "/" && parsed.Path != "/json/version" {
			return fmt.Errorf("HTTP attach URL must be the bare origin or end with /json/version")
		}
		if parsed.User != nil {
			return fmt.Errorf("attach URL must not include userinfo")
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("attach URL must not include query or fragment")
		}
	}

	// Validate host
	host := parsed.Hostname()
	if !isAllowedAttachHost(host, o.runtimeCfg.AttachAllowHosts) {
		return fmt.Errorf("host %q not allowed (allowed: %v)", host, o.runtimeCfg.AttachAllowHosts)
	}

	return nil
}

func isAllowedAttachHost(host string, allowedHosts []string) bool {
	for _, allowed := range allowedHosts {
		if allowed == "*" || host == allowed {
			return true
		}
	}
	return false
}
