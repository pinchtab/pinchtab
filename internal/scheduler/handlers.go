package scheduler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/web"
)

// RegisterHandlers mounts the scheduler API routes on the given mux.
func (s *Scheduler) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /tasks", s.handleSubmit)
	mux.HandleFunc("GET /tasks", s.handleList)
	mux.HandleFunc("GET /tasks/{id}", s.handleGet)
	mux.HandleFunc("POST /tasks/{id}/cancel", s.handleCancel)
}

func (s *Scheduler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, err)
		return
	}

	task, err := s.Submit(req)
	if err != nil {
		if task != nil && task.State == StateRejected {
			stats := s.QueueStats()
			web.ErrorCode(w, 429, "queue_full", err.Error(), true, map[string]any{
				"agentId":     req.AgentID,
				"queued":      stats.TotalQueued,
				"maxQueue":    s.cfg.MaxQueueSize,
				"maxPerAgent": s.cfg.MaxPerAgent,
			})
			return
		}
		web.Error(w, 400, err)
		return
	}

	snap := task.Snapshot()
	web.JSON(w, 202, map[string]any{
		"taskId":    snap.ID,
		"state":     snap.State,
		"position":  snap.Position,
		"createdAt": snap.CreatedAt,
	})
}

func (s *Scheduler) handleGet(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		web.Error(w, 400, nil)
		return
	}

	task := s.GetTask(taskID)
	if task == nil {
		web.ErrorCode(w, 404, "not_found", "task not found", false, nil)
		return
	}

	web.JSON(w, 200, task.Snapshot())
}

func (s *Scheduler) handleCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		web.Error(w, 400, nil)
		return
	}

	if err := s.Cancel(taskID); err != nil {
		if strings.Contains(err.Error(), "terminal state") {
			web.ErrorCode(w, 409, "conflict", err.Error(), false, nil)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			web.ErrorCode(w, 404, "not_found", err.Error(), false, nil)
			return
		}
		web.Error(w, 500, err)
		return
	}

	web.JSON(w, 200, map[string]string{"status": "cancelled", "taskId": taskID})
}

func (s *Scheduler) handleList(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	stateParam := r.URL.Query().Get("state")

	var states []TaskState
	if stateParam != "" {
		for _, s := range strings.Split(stateParam, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				states = append(states, TaskState(trimmed))
			}
		}
	}

	tasks := s.ListTasks(agentID, states)
	if tasks == nil {
		tasks = []Task{}
	}
	web.JSON(w, 200, map[string]any{"tasks": tasks, "count": len(tasks)})
}
