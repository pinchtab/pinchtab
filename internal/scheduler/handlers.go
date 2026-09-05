package scheduler

import (
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// RegisterHandlers mounts the family from the route catalogue rather than from a
// list of its own, so the routes served, the routes documented and the routes the
// front door refuses when the scheduler is off are one list. A catalogued route
// with no handler panics here, the same discipline the bridge's own registration
// keeps.
func (s *Scheduler) RegisterHandlers(mux *http.ServeMux) {
	handlers := map[string]http.HandlerFunc{
		"POST /tasks":             s.handleSubmit,
		"GET /tasks":              s.handleList,
		"GET /tasks/{id}":         s.handleGet,
		"POST /tasks/{id}/cancel": s.handleCancel,
		"POST /tasks/batch":       s.handleBatch,
		"GET /scheduler/stats":    s.handleStats,
	}
	for _, ep := range routes.SchedulerEndpoints() {
		handler, ok := handlers[ep.Route()]
		if !ok {
			panic("scheduler: no handler for catalogued route " + ep.Route())
		}
		mux.HandleFunc(ep.Route(), handler)
	}
}

func (s *Scheduler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req SubmitRequest
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}

	task, err := s.Submit(req)
	if err != nil {
		if task != nil && task.State == StateRejected {
			stats := s.QueueStats()
			httpx.ErrorCode(w, 429, "queue_full", err.Error(), true, map[string]any{
				"agentId":     req.AgentID,
				"queued":      stats.TotalQueued,
				"maxQueue":    s.cfg.MaxQueueSize,
				"maxPerAgent": s.cfg.MaxPerAgent,
			})
			return
		}
		httpx.Error(w, 400, err)
		return
	}

	snap := task.Snapshot()
	httpx.JSON(w, 202, map[string]any{
		"taskId":    snap.ID,
		"state":     snap.State,
		"position":  snap.Position,
		"createdAt": snap.CreatedAt,
	})
}

func (s *Scheduler) handleGet(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		httpx.Error(w, 400, nil)
		return
	}

	task := s.GetTask(taskID)
	if task == nil {
		httpx.ErrorCode(w, 404, "not_found", "task not found", false, nil)
		return
	}

	httpx.JSON(w, 200, task.Snapshot())
}

func (s *Scheduler) handleCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		httpx.Error(w, 400, nil)
		return
	}

	if err := s.Cancel(taskID); err != nil {
		if strings.Contains(err.Error(), "terminal state") {
			httpx.ErrorCode(w, 409, "conflict", err.Error(), false, nil)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			httpx.ErrorCode(w, 404, "not_found", err.Error(), false, nil)
			return
		}
		httpx.Error(w, 500, err)
		return
	}

	httpx.JSON(w, 200, map[string]string{"status": "cancelled", "taskId": taskID})
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
		tasks = []*Task{}
	}
	httpx.JSON(w, 200, map[string]any{"tasks": tasks, "count": len(tasks)})
}

func (s *Scheduler) handleStats(w http.ResponseWriter, _ *http.Request) {
	queue := s.QueueStats()
	metrics := s.GetMetrics()
	httpx.JSON(w, 200, map[string]any{
		"queue":   queue,
		"metrics": metrics,
		"config": map[string]any{
			"strategy":          s.cfg.Strategy,
			"maxQueueSize":      s.cfg.MaxQueueSize,
			"maxPerAgent":       s.cfg.MaxPerAgent,
			"maxInflight":       s.cfg.MaxInflight,
			"maxPerAgentFlight": s.cfg.MaxPerAgentFlight,
			"workerCount":       s.cfg.WorkerCount,
			"resultTTL":         s.cfg.ResultTTL.String(),
		},
	})
}
