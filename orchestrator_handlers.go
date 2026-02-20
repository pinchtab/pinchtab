package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("POST /instances/launch", o.handleLaunch)
	mux.HandleFunc("POST /instances/{id}/stop", o.handleStop)
	mux.HandleFunc("GET /profiles/{name}/instance", o.handleProfileInstance)
	mux.HandleFunc("POST /profiles/{name}/stop", o.handleStopProfile)
	mux.HandleFunc("GET /instances/{id}/logs", o.handleLogs)
	mux.HandleFunc("GET /instances/tabs", o.handleAllTabs)
	mux.HandleFunc("GET /instances/{id}/proxy/screencast", o.handleProxyScreencast)
	mux.HandleFunc("POST /start/{id}", o.handleStartByID)
	mux.HandleFunc("POST /stop/{id}", o.handleStopByID)
}

func (o *Orchestrator) handleList(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, o.List())
}

func (o *Orchestrator) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Port string `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("invalid JSON"))
		return
	}
	if req.Name == "" || req.Port == "" {
		jsonErr(w, 400, fmt.Errorf("name and port required"))
		return
	}

	slog.Info("launch request", "profile", req.Name, "port", req.Port, "headed", true)
	inst, err := o.Launch(req.Name, req.Port, false)
	if isStartingConflict(err) {
		slog.Warn("launch blocked by starting instance; stopping and retrying", "profile", req.Name)
		if stopErr := o.StopProfile(req.Name); stopErr != nil {
			slog.Warn("failed to stop starting instance", "profile", req.Name, "err", stopErr)
			jsonErr(w, 409, err)
			return
		}
		inst, err = o.Launch(req.Name, req.Port, false)
	}
	if err != nil {
		slog.Warn("launch rejected", "profile", req.Name, "port", req.Port, "err", err)
		jsonErr(w, 409, err)
		return
	}
	slog.Info("launch accepted", "id", inst.ID, "profile", inst.Name, "port", inst.Port)
	jsonResp(w, 201, inst)
}

func (o *Orchestrator) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := o.Stop(id); err != nil {
		jsonErr(w, 404, err)
		return
	}
	jsonResp(w, 200, map[string]string{"status": "stopped", "id": id})
}

func (o *Orchestrator) handleProfileInstance(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		jsonErr(w, 400, fmt.Errorf("profile name required"))
		return
	}
	inst, ok := findProfileInstance(o.List(), name)
	if !ok {
		jsonResp(w, 200, map[string]any{
			"name":    name,
			"running": false,
			"status":  "stopped",
			"port":    "",
		})
		return
	}
	jsonResp(w, 200, map[string]any{
		"name":    name,
		"running": inst.Status == "running",
		"status":  inst.Status,
		"port":    inst.Port,
		"id":      inst.ID,
		"pid":     inst.PID,
		"error":   inst.Error,
	})
}

func (o *Orchestrator) handleStopProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		jsonErr(w, 400, fmt.Errorf("profile name required"))
		return
	}
	if err := o.StopProfile(name); err != nil {
		jsonErr(w, 404, err)
		return
	}
	jsonResp(w, 200, map[string]string{"status": "stopped", "name": name})
}

func (o *Orchestrator) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logs, err := o.Logs(id)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(logs))
}

func (o *Orchestrator) handleAllTabs(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, o.AllTabs())
}

func (o *Orchestrator) handleProxyScreencast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tabID := r.URL.Query().Get("tabId")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok || inst.Status != "running" {
		http.Error(w, "instance not found or not running", 404)
		return
	}

	targetURL := fmt.Sprintf("ws://localhost:%s/screencast?tabId=%s", inst.Port, tabID)
	jsonResp(w, 200, map[string]string{"wsUrl": targetURL})
}

func (o *Orchestrator) handleStartByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if o.profiles == nil {
		jsonErr(w, 500, fmt.Errorf("profile manager not configured"))
		return
	}
	name, err := o.profiles.FindByID(id)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	// Optional port from request body; auto-allocate if not provided.
	var req struct {
		Port     string `json:"port"`
		Headless bool   `json:"headless"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	port := req.Port
	if port == "" {
		p, err := findFreePort()
		if err != nil {
			jsonErr(w, 500, fmt.Errorf("failed to allocate port: %w", err))
			return
		}
		port = p
	}

	slog.Info("start-by-id request", "profileId", id, "profile", name, "port", port, "headless", req.Headless)
	inst, err := o.Launch(name, port, req.Headless)
	if isStartingConflict(err) {
		slog.Warn("start-by-id blocked by starting instance; stopping and retrying", "profile", name)
		if stopErr := o.StopProfile(name); stopErr != nil {
			jsonErr(w, 409, err)
			return
		}
		inst, err = o.Launch(name, port, req.Headless)
	}
	if err != nil {
		jsonErr(w, 409, err)
		return
	}
	jsonResp(w, 201, inst)
}

func (o *Orchestrator) handleStopByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if o.profiles == nil {
		jsonErr(w, 500, fmt.Errorf("profile manager not configured"))
		return
	}
	name, err := o.profiles.FindByID(id)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}
	if err := o.StopProfile(name); err != nil {
		jsonErr(w, 404, err)
		return
	}
	jsonResp(w, 200, map[string]string{"status": "stopped", "id": id, "name": name})
}

func findFreePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return fmt.Sprintf("%d", port), nil
}

func isStartingConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already has an active instance (starting)")
}

func findProfileInstance(instances []Instance, name string) (Instance, bool) {
	bestIdx := -1
	bestScore := -1
	for i := range instances {
		if instances[i].Name != name {
			continue
		}
		score := profileStatusScore(instances[i].Status)
		if bestIdx == -1 || score > bestScore || (score == bestScore && instances[i].StartedAt.After(instances[bestIdx].StartedAt)) {
			bestIdx = i
			bestScore = score
		}
	}
	if bestIdx == -1 {
		return Instance{}, false
	}
	return instances[bestIdx], true
}

func profileStatusScore(status string) int {
	switch status {
	case "running":
		return 5
	case "starting":
		return 4
	case "stopping":
		return 3
	case "error":
		return 2
	default:
		return 1
	}
}
