package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/web"
)

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
