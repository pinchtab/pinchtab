package profiles

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (pm *ProfileManager) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /profiles", pm.handleList)
	mux.HandleFunc("POST /profiles", pm.handleCreate)
	mux.HandleFunc("POST /profiles/import", pm.handleImport)
	mux.HandleFunc("POST /profiles/reset", pm.handleReset)
	mux.HandleFunc("DELETE /profiles", pm.handleDelete)
	mux.HandleFunc("GET /profiles/logs", pm.handleLogs)
	mux.HandleFunc("GET /profiles/analytics", pm.handleAnalytics)
	mux.HandleFunc("PATCH /profiles/meta", pm.handleUpdateMeta)
}

func (pm *ProfileManager) handleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := pm.List()
	if err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, profiles)
}

func (pm *ProfileManager) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		UseWhen     string `json:"useWhen"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, err)
		return
	}
	if req.Name == "" {
		web.Error(w, 400, fmt.Errorf("name required"))
		return
	}

	meta := ProfileMeta{
		Description: req.Description,
		UseWhen:     req.UseWhen,
	}

	if err := pm.CreateWithMeta(req.Name, meta); err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "created", "name": req.Name})
}

func (pm *ProfileManager) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		SourcePath  string `json:"sourcePath"`
		Description string `json:"description"`
		UseWhen     string `json:"useWhen"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, err)
		return
	}
	if req.Name == "" || req.SourcePath == "" {
		web.Error(w, 400, fmt.Errorf("name and sourcePath required"))
		return
	}

	meta := ProfileMeta{
		Description: req.Description,
		UseWhen:     req.UseWhen,
	}

	if err := pm.ImportWithMeta(req.Name, req.SourcePath, meta); err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "imported", "name": req.Name})
}

func (pm *ProfileManager) handleReset(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		web.Error(w, 400, fmt.Errorf("name required"))
		return
	}
	if err := pm.Reset(name); err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "reset", "name": name})
}

func (pm *ProfileManager) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		web.Error(w, 400, fmt.Errorf("name required"))
		return
	}
	if err := pm.Delete(name); err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "deleted", "name": name})
}

func (pm *ProfileManager) handleLogs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		web.Error(w, 400, fmt.Errorf("name required"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs := pm.Logs(name, limit)
	web.JSON(w, 200, logs)
}

func (pm *ProfileManager) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		web.Error(w, 400, fmt.Errorf("name required"))
		return
	}
	report := pm.Analytics(name)
	web.JSON(w, 200, report)
}

func (pm *ProfileManager) handleUpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		UseWhen     string `json:"useWhen"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, err)
		return
	}
	if req.Name == "" {
		web.Error(w, 400, fmt.Errorf("name required"))
		return
	}

	updates := make(map[string]string)
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.UseWhen != "" {
		updates["useWhen"] = req.UseWhen
	}

	if err := pm.UpdateMeta(req.Name, updates); err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, map[string]string{"status": "updated", "name": req.Name})
}
