package profiles

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (pm *ProfileManager) RegisterHandlers(mux *http.ServeMux) {
	// Standard CRUD endpoints
	mux.HandleFunc("GET /profiles", pm.handleList)
	mux.HandleFunc("POST /profiles", pm.handleCreate)        // RESTful: POST /profiles instead of /profiles/create
	mux.HandleFunc("POST /profiles/create", pm.handleCreate) // Backward compat
	mux.HandleFunc("GET /profiles/{id}", pm.handleGetByID)   // Get single profile by ID or name

	// Advanced endpoints (specific paths, no conflict with {id})
	mux.HandleFunc("POST /profiles/import", pm.handleImport)
	mux.HandleFunc("PATCH /profiles/meta", pm.handleUpdateMeta)
	mux.HandleFunc("POST /profiles/{id}/reset", pm.handleResetByIDOrName)        // Reset profile
	mux.HandleFunc("GET /profiles/{id}/logs", pm.handleLogsByIDOrName)           // Get logs
	mux.HandleFunc("GET /profiles/{id}/analytics", pm.handleAnalyticsByIDOrName) // Get analytics
	mux.HandleFunc("DELETE /profiles/{id}", pm.handleDeleteByID)                 // Delete profile
	mux.HandleFunc("PATCH /profiles/{id}", pm.handleUpdateByIDOrName)            // Update profile metadata
}

func (pm *ProfileManager) handleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := pm.List()
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	// Filter out temporary profiles by default (unless requested with ?all=true)
	showAll := r.URL.Query().Get("all") == "true"
	if !showAll {
		filtered := []map[string]any{}
		for _, p := range profiles {
			if !p.Temporary {
				// Convert to map for JSON response
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

	// Generate and return the profile ID
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

// New RESTful handlers

func (pm *ProfileManager) handleGetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	profiles, err := pm.List()
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	var foundProfile map[string]any

	for _, p := range profiles {
		// Match by ID or name (backward-compatible)
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

	// Find profile name by ID
	name, err := pm.FindByID(id)
	if err != nil {
		// Try as name directly (backward compat)
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

// Helper to find profile name by ID or use as name directly
func (pm *ProfileManager) resolveIDOrName(idOrName string) (string, error) {
	// Try as ID first
	name, err := pm.FindByID(idOrName)
	if err == nil {
		return name, nil
	}
	// Try as name directly
	if pm.Exists(idOrName) {
		return idOrName, nil
	}
	return "", fmt.Errorf("profile %q not found (not a valid ID or name)", idOrName)
}

// Consolidated handlers that work with both ID and name

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
