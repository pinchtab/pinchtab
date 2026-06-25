package profiles

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type profileResponse struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Path              string    `json:"path"`
	PathExists        bool      `json:"pathExists"`
	Created           time.Time `json:"created"`
	LastUsed          time.Time `json:"lastUsed"`
	DiskUsage         int64     `json:"diskUsage"`
	SizeMB            float64   `json:"sizeMB"`
	Running           bool      `json:"running"`
	Source            string    `json:"source"`
	ChromeProfileName string    `json:"chromeProfileName"`
	AccountEmail      string    `json:"accountEmail"`
	AccountName       string    `json:"accountName"`
	HasAccount        bool      `json:"hasAccount"`
	UseWhen           string    `json:"useWhen"`
	Description       string    `json:"description"`
}

func newProfileResponse(p bridge.ProfileInfo) profileResponse {
	return profileResponse{
		ID:                p.ID,
		Name:              p.Name,
		Path:              p.Path,
		PathExists:        p.PathExists,
		Created:           p.Created,
		LastUsed:          p.LastUsed,
		DiskUsage:         p.DiskUsage,
		SizeMB:            float64(p.DiskUsage) / (1024 * 1024),
		Running:           p.Running,
		Source:            p.Source,
		ChromeProfileName: p.ChromeProfileName,
		AccountEmail:      p.AccountEmail,
		AccountName:       p.AccountName,
		HasAccount:        p.HasAccount,
		UseWhen:           p.UseWhen,
		Description:       p.Description,
	}
}

func profileMutationStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrInvalidProfileName):
		return http.StatusBadRequest
	case errors.Is(err, ErrProfileExists), errors.Is(err, ErrProfileDirExists):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

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
	mux.HandleFunc("PATCH /profiles/{id}", pm.handleUpdateByID)
}

func (pm *ProfileManager) handleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := pm.List()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	showAll := r.URL.Query().Get("all") == "true"
	if !showAll {
		filtered := []profileResponse{}
		for _, p := range profiles {
			if !p.Temporary {
				filtered = append(filtered, newProfileResponse(p))
			}
		}
		httpx.JSON(w, 200, filtered)
		return
	}

	httpx.JSON(w, 200, profiles)
}

func (pm *ProfileManager) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		UseWhen     string `json:"useWhen"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if req.Name == "" {
		httpx.Error(w, 400, fmt.Errorf("name required"))
		return
	}

	meta := ProfileMeta{
		Description: req.Description,
		UseWhen:     req.UseWhen,
	}

	if err := pm.CreateWithMeta(req.Name, meta); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}

	generatedID := profileID(req.Name)
	authn.AuditLog(r, "profile.created", "profileId", generatedID, "profileName", req.Name)
	httpx.JSON(w, 200, map[string]any{
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
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if req.Name == "" || req.SourcePath == "" {
		httpx.Error(w, 400, fmt.Errorf("name and sourcePath required"))
		return
	}

	meta := ProfileMeta{
		Description: req.Description,
		UseWhen:     req.UseWhen,
	}

	if err := pm.ImportWithMeta(req.Name, req.SourcePath, meta); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}
	authn.AuditLog(r, "profile.imported", "profileName", req.Name)
	httpx.JSON(w, 200, map[string]string{"status": "imported", "name": req.Name})
}

func (pm *ProfileManager) handleUpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		UseWhen     *string `json:"useWhen"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if req.Name == "" {
		httpx.Error(w, 400, fmt.Errorf("name required"))
		return
	}

	updates := make(map[string]string)
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.UseWhen != nil {
		updates["useWhen"] = *req.UseWhen
	}

	if err := pm.UpdateMeta(req.Name, updates); err != nil {
		httpx.Error(w, profileMutationStatus(err), err)
		return
	}
	authn.AuditLog(r, "profile.meta_updated", "profileName", req.Name)
	httpx.JSON(w, 200, map[string]string{"status": "updated", "name": req.Name})
}

func (pm *ProfileManager) handleGetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	profiles, err := pm.List()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	for _, p := range profiles {
		if p.ID != id && p.Name != id {
			continue
		}
		httpx.JSON(w, 200, newProfileResponse(p))
		return
	}

	httpx.Error(w, 404, fmt.Errorf("profile %q not found", id))
}

func (pm *ProfileManager) handleDeleteByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	name, err := pm.resolveIDOnly(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	if err := pm.Delete(name); err != nil {
		httpx.Error(w, 500, err)
		return
	}

	authn.AuditLog(r, "profile.deleted", "profileId", id, "profileName", name)
	httpx.JSON(w, 200, map[string]any{"status": "deleted", "id": id, "name": name})
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

func (pm *ProfileManager) resolveIDOnly(id string) (string, error) {
	name, err := pm.FindByID(id)
	if err != nil {
		return "", fmt.Errorf("profile %q not found (must use profile ID, not name)", id)
	}
	return name, nil
}

func (pm *ProfileManager) handleUpdateByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := pm.resolveIDOnly(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	var req struct {
		Name        *string `json:"name"`
		UseWhen     *string `json:"useWhen"`
		Description *string `json:"description"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
		return
	}

	finalName := name
	if req.Name != nil && *req.Name != name {
		if err := pm.Rename(name, *req.Name); err != nil {
			httpx.Error(w, profileMutationStatus(err), err)
			return
		}
		finalName = *req.Name
	}

	updates := make(map[string]string)
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.UseWhen != nil {
		updates["useWhen"] = *req.UseWhen
	}
	if len(updates) > 0 {
		if err := pm.UpdateMeta(finalName, updates); err != nil {
			httpx.Error(w, profileMutationStatus(err), err)
			return
		}
	}

	authn.AuditLog(r, "profile.updated", "profileId", profileID(finalName), "profileName", finalName)
	httpx.JSON(w, 200, map[string]any{"status": "updated", "id": profileID(finalName), "name": finalName})
}

func (pm *ProfileManager) handleResetByIDOrName(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := pm.resolveIDOnly(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	if err := pm.Reset(name); err != nil {
		httpx.Error(w, 500, err)
		return
	}
	authn.AuditLog(r, "profile.reset", "profileId", id, "profileName", name)
	httpx.JSON(w, 200, map[string]any{"status": "reset", "id": id, "name": name})
}

func (pm *ProfileManager) handleLogsByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs := pm.Logs(name, limit)
	httpx.JSON(w, 200, logs)
}

func (pm *ProfileManager) handleAnalyticsByIDOrName(w http.ResponseWriter, r *http.Request) {
	idOrName := r.PathValue("id")
	name, err := pm.resolveIDOrName(idOrName)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	report := pm.Analytics(name)
	httpx.JSON(w, 200, report)
}
