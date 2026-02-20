package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /launch", o.handleLaunch)
	mux.HandleFunc("POST /stop", o.handleStop)
	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("GET /instances/logs", o.handleLogs)
}

func (o *Orchestrator) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile  string `json:"profile"`
		Port     string `json:"port"`
		Headless bool   `json:"headless"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, err)
		return
	}

	if req.Profile == "" {
		web.Error(w, 400, fmt.Errorf("profile name required"))
		return
	}

	// Resolve profile ID if provided
	if len(req.Profile) == 12 {
		if pm := o.profiles; pm != nil {
			if name, err := pm.FindByID(req.Profile); err == nil {
				req.Profile = name
			}
		}
	}

	if req.Port == "" {
		req.Port = "0"
	}

	inst, err := o.Launch(req.Profile, req.Port, req.Headless)
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	web.JSON(w, 200, inst)
}

func (o *Orchestrator) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	profile := r.URL.Query().Get("profile")

	if id != "" {
		if err := o.Stop(id); err != nil {
			web.Error(w, 500, err)
			return
		}
	} else if profile != "" {
		// Resolve profile ID if provided
		if len(profile) == 12 {
			if pm := o.profiles; pm != nil {
				if name, err := pm.FindByID(profile); err == nil {
					profile = name
				}
			}
		}
		if err := o.StopProfile(profile); err != nil {
			web.Error(w, 500, err)
			return
		}
	} else {
		web.Error(w, 400, fmt.Errorf("id or profile required"))
		return
	}

	web.JSON(w, 200, map[string]string{"status": "stopped"})
}

func (o *Orchestrator) handleList(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, o.List())
}

func (o *Orchestrator) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		web.Error(w, 400, fmt.Errorf("id required"))
		return
	}
	logs, err := o.Logs(id)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	web.JSON(w, 200, map[string]string{"id": id, "logs": logs})
}
