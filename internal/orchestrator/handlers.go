package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /profiles/{id}/start", o.handleStartByID)
	mux.HandleFunc("POST /profiles/{id}/stop", o.handleStopByID)
	mux.HandleFunc("GET /profiles/{id}/instance", o.handleProfileInstance)

	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("GET /instances/{id}", o.handleGetInstance)
	mux.HandleFunc("GET /instances/tabs", o.handleAllTabs)
	mux.HandleFunc("POST /instances/start", o.handleStartInstance)
	mux.HandleFunc("POST /instances/launch", o.handleLaunchByName)
	mux.HandleFunc("POST /instances/{id}/start", o.handleStartByInstanceID)
	mux.HandleFunc("POST /instances/{id}/stop", o.handleStopByInstanceID)
	mux.HandleFunc("GET /instances/{id}/logs", o.handleLogsByID)
	mux.HandleFunc("GET /instances/{id}/tabs", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/proxy/screencast", o.handleProxyScreencast)
	mux.HandleFunc("POST /instances/{id}/tabs/open", o.handleInstanceTabOpen)
	mux.HandleFunc("POST /instances/{id}/tab", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/screencast", o.proxyToInstance)

	mux.HandleFunc("POST /tabs/{id}/close", o.handleTabClose)
	mux.HandleFunc("POST /tabs/{id}/navigate", o.handleTabNavigate)
	mux.HandleFunc("GET /tabs/{id}/snapshot", o.handleTabSnapshot)
	mux.HandleFunc("GET /tabs/{id}/screenshot", o.handleTabScreenshot)
	mux.HandleFunc("POST /tabs/{id}/action", o.handleTabAction)
	mux.HandleFunc("POST /tabs/{id}/actions", o.handleTabActions)
	mux.HandleFunc("GET /tabs/{id}/text", o.handleTabText)
	mux.HandleFunc("POST /tabs/{id}/evaluate", o.handleTabEvaluate)
	mux.HandleFunc("GET /tabs/{id}/pdf", o.handleTabPDF)
	mux.HandleFunc("POST /tabs/{id}/pdf", o.handleTabPDF)
	mux.HandleFunc("GET /tabs/{id}/download", o.handleTabDownload)
	mux.HandleFunc("POST /tabs/{id}/upload", o.handleTabUpload)
	mux.HandleFunc("POST /tabs/{id}/lock", o.handleTabLock)
	mux.HandleFunc("POST /tabs/{id}/unlock", o.handleTabUnlock)
	mux.HandleFunc("GET /tabs/{id}/cookies", o.handleTabGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", o.handleTabSetCookies)
}

func (o *Orchestrator) handleList(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, o.List())
}

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

func (o *Orchestrator) resolveProfileName(idOrName string) (string, error) {
	if o.profiles == nil {
		return "", fmt.Errorf("profile manager not configured")
	}
	if name, err := o.profiles.FindByID(idOrName); err == nil {
		return name, nil
	}
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

	inst, err := o.Launch(name, req.Port, req.Headless)
	if err != nil {
		statusCode := classifyLaunchError(err)
		web.Error(w, statusCode, err)
		return
	}
	web.JSON(w, 201, inst)
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
		ProfileId string `json:"profileId,omitempty"`
		Name      string `json:"name,omitempty"`
		Mode      string `json:"mode"`
		Port      string `json:"port,omitempty"`
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

	inst, err := o.Launch(name, req.Port, headless)
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

	started, err := o.Launch(profileName, port, headless)
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
		ProfileID string `json:"profileId,omitempty"`
		Mode      string `json:"mode,omitempty"`
		Port      string `json:"port,omitempty"`
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

	inst, err := o.Launch(profileName, req.Port, headless)
	if err != nil {
		statusCode := classifyLaunchError(err)
		web.Error(w, statusCode, err)
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

	targetPath := r.URL.Path
	if len(targetPath) > len("/instances/"+id) {
		targetPath = targetPath[len("/instances/"+id):]
	} else {
		targetPath = ""
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     targetPath,
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) proxyToURL(w http.ResponseWriter, r *http.Request, targetURL *url.URL) {
	if targetURL.Hostname() != "localhost" {
		web.Error(w, 400, fmt.Errorf("invalid proxy target: only localhost allowed"))
		return
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("failed to create proxy request: %w", err))
		return
	}

	for key, values := range r.Header {
		switch key {
		case "Host", "Connection", "Keep-Alive", "Proxy-Authenticate",
			"Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade":
		default:
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	resp, err := o.client.Do(proxyReq)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("failed to proxy to instance: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(readResponseBody(resp))
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
		tabs, err := o.fetchTabs(inst.URL)
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

func (o *Orchestrator) handleTabClose(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	// Construct request body for the bridge's /tab endpoint
	reqBody, _ := json.Marshal(map[string]string{
		"action": "close",
		"tabId":  tabID,
	})

	targetURL := fmt.Sprintf("http://localhost:%s/tab", inst.Port)
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		web.Error(w, 500, err)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (o *Orchestrator) handleTabNavigate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/navigate",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/snapshot",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/screenshot",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabAction(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/action",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabActions(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/actions",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabText(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/text",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabEvaluate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/evaluate",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabPDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/pdf",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabDownload(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/download",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabUpload(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/upload",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabLock(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/lock",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabUnlock(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/unlock",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabGetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/cookies",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleTabSetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tabs/" + tabID + "/cookies",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, r, targetURL)
}

func (o *Orchestrator) handleInstanceTabOpen(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		URL string `json:"url,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	payload, err := json.Marshal(map[string]any{
		"action": "new",
		"url":    req.URL,
	})
	if err != nil {
		web.Error(w, 500, fmt.Errorf("failed to build tab open request: %w", err))
		return
	}

	proxyReq := r.Clone(r.Context())
	proxyReq.Body = io.NopCloser(bytes.NewReader(payload))
	proxyReq.ContentLength = int64(len(payload))
	proxyReq.Header = r.Header.Clone()
	proxyReq.Header.Set("Content-Type", "application/json")

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tab",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, proxyReq, targetURL)
}
