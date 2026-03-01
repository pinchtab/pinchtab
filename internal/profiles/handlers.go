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
	mux.HandleFunc("POST /profiles/create", pm.handleCreate)
	mux.HandleFunc("GET /profiles/{id}", pm.handleGetByID)

	mux.HandleFunc("POST /profiles/import", pm.handleImport)
	mux.HandleFunc("PATCH /profiles/meta", pm.handleUpdateMeta)
	mux.HandleFunc("POST /profiles/{id}/reset", pm.handleResetByIDOrName)
	mux.HandleFunc("GET /profiles/{id}/logs", pm.handleLogsByIDOrName)
	mux.HandleFunc("GET /profiles/{id}/analytics", pm.handleAnalyticsByIDOrName)
	mux.HandleFunc("DELETE /profiles/{id}", pm.handleDeleteByID)
	mux.HandleFunc("PATCH /profiles/{id}", pm.handleUpdateByIDOrName)
}

func (pm *ProfileManager) handleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := pm.List()
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	showAll := r.URL.Query().Get("all") == "true"
	if !showAll {
		filtered := []map[string]any{}
		for _, p := range profiles {
			if !p.Temporary {
				sizeMB := float64(p.DiskUsage) / (1024 * 1024)
				filtered = append(filtered, map[string]any{
					"id":                p.ID,
					"name":              p.Name,
					"path":              p.Path,
					"pathExists":        p.PathExists,
					"created":           p.Created,
					"lastUsed":          p.LastUsed,
					"diskUsage":         p.DiskUsage,
					"sizeMB":            sizeMB,
					"running":           p.Running,
					"source":            p.Source,
					"chromeProfileName": p.ChromeProfileName,
					"accountEmail":      p.AccountEmail,
					"accountName":       p.AccountName,
					"hasAccount":        p.HasAccount,
					"useWhen":           p.UseWhen,
					"description":       p.Description,
				})
			}
		}
		web.JSON(w, 200, filtered)
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

	generatedID := profileID(req.Name)
	web.JSON(w, 200, map[string]any{
		"status": "created",
		"id":     generatedID,
		"name":   req.Name,
	})
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

func (pm *ProfileManager) handleGetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	profiles, err := pm.List()
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	var foundProfile map[string]any

	for _, p := range profiles {
		if p.ID != id && p.Name != id {
			continue
		}
		foundProfile = map[string]any{
			"id":                p.ID,
			"name":              p.Name,
			"path":              p.Path,
			"pathExists":        p.PathExists,
			"created":           p.Created,
			"diskUsage":         p.DiskUsage,
			"sizeMB":            float64(p.DiskUsage) / (1024 * 1024),
			"source":            p.Source,
			"chromeProfileName": p.ChromeProfileName,
			"accountEmail":      p.AccountEmail,
			"accountName":       p.AccountName,
			"hasAccount":        p.HasAccount,
			"useWhen":           p.UseWhen,
			"description":       p.Description,
		}
		break
	}

	if foundProfile == nil {
		web.Error(w, 404, fmt.Errorf("profile %q not found", id))
		return
	}

	web.JSON(w, 200, foundProfile)
}

func (pm *ProfileManager) handleDeleteByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	name, err := pm.FindByID(id)
	if err != nil {
		name = id
		if !pm.Exists(name) {
			web.Error(w, 404, fmt.Errorf("profile %q not found", id))
			return
		}
	}

	if err := pm.Delete(name); err != nil {
		web.Error(w, 404, err)
		return
	}

	web.JSON(w, 200, map[string]any{"status": "deleted", "id": id, "name": name})
}

func (pm *ProfileManager) resolveIDOrName(idOrName string) (string, error) {
	name, err := pm.FindByID(idOrName)
	if err == nil {
		return name, nil
	}
	if pm.Exists(idOrName) {
		return idOrName, nil
	}
	return "", fmt.Errorf("profile %q not found (not a valid ID or name)", idOrName)
}

func (pm *ProfileManager) handleUpdateByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	var req struct {
		Name        string `json:"name"`
		UseWhen     string `json:"useWhen"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("invalid JSON"))
		return
	}

	updates := make(map[string]string)
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.UseWhen != "" {
		updates["useWhen"] = req.UseWhen
	}
	if len(updates) > 0 {
		if err := pm.UpdateMeta(name, updates); err != nil {
			web.Error(w, 404, err)
			return
		}
	}

	web.JSON(w, 200, map[string]any{"status": "updated", "id": idOrName, "name": name})
}

func (pm *ProfileManager) handleResetByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	if err := pm.Reset(name); err != nil {
		web.Error(w, 404, err)
		return
	}
	web.JSON(w, 200, map[string]any{"status": "reset", "id": idOrName, "name": name})
}

func (pm *ProfileManager) handleLogsByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs := pm.Logs(name, limit)
	web.JSON(w, 200, logs)
}

func (pm *ProfileManager) handleAnalyticsByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	report := pm.Analytics(name)
	web.JSON(w, 200, report)
}
