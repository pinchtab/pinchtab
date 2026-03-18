package scheduler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

var (
	errMissingAgentID = errors.New("missing required field 'agentId'")
	errEmptyBatch     = errors.New("batch must contain at least one task")
)

// BatchRequest is the JSON body for POST /tasks/batch.
type BatchRequest struct {
	AgentID     string         `json:"agentId"`
	CallbackURL string         `json:"callbackUrl,omitempty"`
	Tasks       []BatchTaskDef `json:"tasks"`
}

// BatchTaskDef defines a single task inside a batch.
type BatchTaskDef struct {
	Action   string         `json:"action"`
	TabID    string         `json:"tabId,omitempty"`
	Ref      string         `json:"ref,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	Priority int            `json:"priority,omitempty"`
	Deadline string         `json:"deadline,omitempty"`
}

// BatchResponseItem is the result for each submitted task in the batch.
type BatchResponseItem struct {
	TaskID   string    `json:"taskId"`
	State    TaskState `json:"state"`
	Position int       `json:"position,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func (s *Scheduler) handleBatch(w http.ResponseWriter, r *http.Request) {
	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, err)
		return
	}

	if req.AgentID == "" {
		httpx.Error(w, 400, errMissingAgentID)
		return
	}
	if len(req.Tasks) == 0 {
		httpx.Error(w, 400, errEmptyBatch)
		return
	}
	s.cfgMu.RLock()
	maxBatch := s.cfg.MaxBatchSize
	s.cfgMu.RUnlock()
	if len(req.Tasks) > maxBatch {
		httpx.ErrorCode(w, 400, "batch_too_large", "batch exceeds maximum size", false, map[string]any{
			"submitted": len(req.Tasks),
			"max":       maxBatch,
		})
		return
	}

	results := make([]BatchResponseItem, 0, len(req.Tasks))
	for _, td := range req.Tasks {
		sr := SubmitRequest{
			AgentID:     req.AgentID,
			Action:      td.Action,
			TabID:       td.TabID,
			Ref:         td.Ref,
			Params:      td.Params,
			Priority:    td.Priority,
			Deadline:    td.Deadline,
			CallbackURL: req.CallbackURL,
		}

		task, err := s.Submit(sr)
		if err != nil {
			item := BatchResponseItem{State: StateRejected, Error: err.Error()}
			if task != nil {
				item.TaskID = task.ID
			}
			results = append(results, item)
			slog.Warn("batch: task rejected", "agent", req.AgentID, "action", td.Action, "err", err)
			continue
		}

		results = append(results, BatchResponseItem{
			TaskID:   task.ID,
			State:    task.GetState(),
			Position: task.Position,
		})
	}

	httpx.JSON(w, 202, map[string]any{
		"tasks":     results,
		"submitted": len(results),
	})
}
