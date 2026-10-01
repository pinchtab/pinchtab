package orchestrator

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
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
		httpx.Error(w, 404, err)
		return
	}

	var req struct {
		Port            string                 `json:"port,omitempty"`
		Headless        bool                   `json:"headless"`
		SecurityPolicy  *bridge.SecurityPolicy `json:"securityPolicy,omitempty"`
		Browser         string                 `json:"browser,omitempty"`
		FallbackTargets []string               `json:"fallbackTargets,omitempty"`
	}
	if err := httpx.DecodeOptionalJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}
	if err := validateStartInstanceSecurityPolicy(req.SecurityPolicy); err != nil {
		httpx.Error(w, 400, err)
		return
	}

	inst, err := o.LaunchWithTargetSelection(name, req.Port, req.Headless, req.Browser, req.FallbackTargets, LaunchOptions{
		SecurityPolicy: req.SecurityPolicy,
		Browser:        req.Browser,
	})
	if err != nil {
		writeLaunchError(w, err)
		return
	}
	authn.AuditLog(r, "instance.started", "profileId", id, "profileName", name, "instanceId", inst.ID)
	httpx.JSON(w, 201, inst)
}

func (o *Orchestrator) handleStopByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := o.resolveProfileName(id)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if err := o.StopProfile(name); err != nil {
		httpx.Error(w, 404, err)
		return
	}
	authn.AuditLog(r, "instance.stopped", "profileId", id, "profileName", name)
	httpx.JSON(w, 200, map[string]string{"status": "stopped", "id": id, "name": name})
}

type profileInstanceResponse struct {
	Exists  bool                  `json:"exists"`
	ID      string                `json:"id,omitempty"`
	Message string                `json:"message,omitempty"`
	Name    string                `json:"name"`
	Port    string                `json:"port"`
	Running bool                  `json:"running"`
	Status  bridge.InstanceStatus `json:"status"`
}

const profileMissingStatus bridge.InstanceStatus = "missing"

func (o *Orchestrator) handleProfileInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := o.resolveProfileName(id)
	if err != nil {
		httpx.JSON(w, 200, profileInstanceResponse{
			Name:    id,
			Status:  profileMissingStatus,
			Message: fmt.Sprintf("Profile %q does not exist. Creating and authenticating a reusable profile is a human setup step.", id),
		})
		return
	}

	for _, inst := range o.List() {
		if inst.ProfileName == name && inst.Status.IsActive() {
			httpx.JSON(w, 200, profileInstanceResponse{
				Exists:  true,
				ID:      inst.ID,
				Name:    name,
				Port:    inst.Port,
				Running: inst.Status == bridge.InstanceRunning,
				Status:  inst.Status,
			})
			return
		}
	}
	httpx.JSON(w, 200, profileInstanceResponse{Exists: true, Name: name, Status: bridge.InstanceStopped})
}
