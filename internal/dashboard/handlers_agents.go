package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (d *Dashboard) handleAgents(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, d.Agents())
}

func (d *Dashboard) handleAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	agent, ok := d.Agent(agentID)
	if !ok {
		httpx.ErrorCode(w, http.StatusNotFound, "agent_not_found", "agent not found", false, nil)
		return
	}

	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "both"
	}
	if mode != "tool_calls" && mode != "progress" && mode != "both" {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_mode", "mode must be tool_calls, progress, or both", false, nil)
		return
	}

	httpx.JSON(w, http.StatusOK, apiTypes.AgentDetail{
		Agent:  agent,
		Events: d.EventsForAgent(agentID, mode),
	})
}

func (d *Dashboard) handleAgentEventsByID(w http.ResponseWriter, r *http.Request) {
	pathAgentID := agentIDOrAnonymous(r.PathValue("id"))
	evt, ok := d.decodeProgressEvent(w, r, pathAgentID)
	if !ok {
		return
	}
	if evt.AgentID != "" && evt.AgentID != pathAgentID {
		httpx.ErrorCode(w, http.StatusBadRequest, "agent_mismatch", "agentId must match route parameter", false, nil)
		return
	}
	evt.AgentID = pathAgentID
	d.RecordEvent(evt)
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "ok", "id": evt.ID})
}

func (d *Dashboard) decodeProgressEvent(w http.ResponseWriter, r *http.Request, fallbackAgentID string) (apiTypes.ActivityEvent, bool) {
	var evt apiTypes.ActivityEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&evt); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_agent_event", "invalid activity event payload", false, nil)
		return apiTypes.ActivityEvent{}, false
	}

	evt.AgentID = strings.TrimSpace(evt.AgentID)
	if evt.AgentID == "" {
		evt.AgentID = agentIDOrAnonymous(fallbackAgentID)
	}
	evt.Message = strings.TrimSpace(evt.Message)
	if evt.AgentID == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_agent_id", "agentId is required", false, nil)
		return apiTypes.ActivityEvent{}, false
	}
	if evt.Message == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_message", "message is required", false, nil)
		return apiTypes.ActivityEvent{}, false
	}
	if evt.Channel != "" && evt.Channel != "progress" {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_channel", "channel must be progress", false, nil)
		return apiTypes.ActivityEvent{}, false
	}

	evt.Channel = "progress"
	evt.Type = "progress"
	return evt, true
}
