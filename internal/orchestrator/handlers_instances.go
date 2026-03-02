package orchestrator

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

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
