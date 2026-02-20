package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (pm *ProfileManager) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /profiles", pm.handleList)
	mux.HandleFunc("POST /profiles/import", pm.handleImport)
	mux.HandleFunc("POST /profiles/create", pm.handleCreate)
	mux.HandleFunc("PATCH /profiles/{name}", pm.handleUpdate)
	mux.HandleFunc("POST /profiles/{name}/reset", pm.handleReset)
	mux.HandleFunc("DELETE /profiles/{name}", pm.handleDelete)
	mux.HandleFunc("GET /profiles/{name}/logs", pm.handleLogs)
	mux.HandleFunc("GET /profiles/{name}/analytics", pm.handleAnalytics)
}

func (pm *ProfileManager) handleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := pm.List()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err)
		return
	}
	jsonResp(w, http.StatusOK, profiles)
}

func (pm *ProfileManager) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Source      string `json:"source"`
		UseWhen     string `json:"useWhen"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON"))
		return
	}
	if req.Name == "" || req.Source == "" {
		jsonErr(w, http.StatusBadRequest, fmt.Errorf("name and source required"))
		return
	}

	// Import with metadata if provided
	if req.UseWhen != "" || req.Description != "" {
		meta := ProfileMeta{
			UseWhen:     req.UseWhen,
			Description: req.Description,
		}
		if err := pm.ImportWithMeta(req.Name, req.Source, meta); err != nil {
			jsonErr(w, http.StatusConflict, err)
			return
		}
	} else {
		if err := pm.Import(req.Name, req.Source); err != nil {
			jsonErr(w, http.StatusConflict, err)
			return
		}
	}
	jsonResp(w, http.StatusCreated, map[string]string{"status": "imported", "name": req.Name})
}

func (pm *ProfileManager) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		UseWhen     string `json:"useWhen"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON"))
		return
	}
	if req.Name == "" {
		jsonErr(w, http.StatusBadRequest, fmt.Errorf("name required"))
		return
	}

	// Create with metadata if provided
	if req.UseWhen != "" || req.Description != "" {
		meta := ProfileMeta{
			UseWhen:     req.UseWhen,
			Description: req.Description,
		}
		if err := pm.CreateWithMeta(req.Name, meta); err != nil {
			jsonErr(w, http.StatusConflict, err)
			return
		}
	} else {
		if err := pm.Create(req.Name); err != nil {
			jsonErr(w, http.StatusConflict, err)
			return
		}
	}
	jsonResp(w, http.StatusCreated, map[string]string{"status": "created", "name": req.Name})
}

func (pm *ProfileManager) handleReset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := pm.Reset(name); err != nil {
		jsonErr(w, http.StatusNotFound, err)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"status": "reset", "name": name})
}

func (pm *ProfileManager) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := pm.Delete(name); err != nil {
		jsonErr(w, http.StatusNotFound, err)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

func (pm *ProfileManager) handleLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	limit := profileQueryInt(r, "limit", 100)
	logs := pm.Logs(name, limit)
	jsonResp(w, http.StatusOK, logs)
}

func (pm *ProfileManager) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	report := pm.Analytics(name)
	jsonResp(w, http.StatusOK, report)
}

func (pm *ProfileManager) handleUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Name        string `json:"name"`
		UseWhen     string `json:"useWhen"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON"))
		return
	}

	meta := map[string]string{
		"useWhen":     req.UseWhen,
		"description": req.Description,
	}
	if err := pm.UpdateMeta(name, meta); err != nil {
		jsonErr(w, http.StatusNotFound, err)
		return
	}

	if req.Name != "" && req.Name != name {
		if err := pm.Rename(name, req.Name); err != nil {
			jsonErr(w, http.StatusConflict, err)
			return
		}
		name = req.Name
	}

	// Return updated profile info
	info, err := pm.profileInfo(name)
	if err != nil {
		jsonErr(w, http.StatusNotFound, err)
		return
	}
	jsonResp(w, http.StatusOK, info)
}

func (pm *ProfileManager) TrackingMiddleware(profileName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)

		rec := ActionRecord{
			Timestamp:  start,
			Method:     r.Method,
			Endpoint:   r.URL.Path,
			URL:        r.URL.Query().Get("url"),
			TabID:      r.URL.Query().Get("tabId"),
			DurationMs: time.Since(start).Milliseconds(),
			Status:     sw.code,
		}
		pm.tracker.Record(profileName, rec)
	})
}

func profileQueryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}
