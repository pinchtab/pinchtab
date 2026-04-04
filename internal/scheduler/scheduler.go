package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
)

// InstanceResolver finds the localhost port for a given tab ID.
type InstanceResolver interface {
	ResolveTabInstance(tabID string) (port string, err error)
}

// Config holds scheduler tuning knobs.
type Config struct {
	Enabled           bool          `json:"enabled"`
	Strategy          string        `json:"strategy"`
	MaxQueueSize      int           `json:"maxQueueSize"`
	MaxPerAgent       int           `json:"maxPerAgent"`
	MaxInflight       int           `json:"maxInflight"`
	MaxPerAgentFlight int           `json:"maxPerAgentInflight"`
	ResultTTL         time.Duration `json:"resultTTL"`
	WorkerCount       int           `json:"workerCount"`
	MaxBatchSize      int           `json:"maxBatchSize"`
	WatcherInterval   time.Duration `json:"watcherInterval"`
}

// DefaultConfig returns safe defaults.
func DefaultConfig() Config {
	return Config{
		Strategy:          "fair-fifo",
		MaxQueueSize:      1000,
		MaxPerAgent:       100,
		MaxInflight:       20,
		MaxPerAgentFlight: 10,
		ResultTTL:         5 * time.Minute,
		WorkerCount:       4,
		MaxBatchSize:      50,
		WatcherInterval:   30 * time.Second,
	}
}

// Scheduler is the core dispatch engine.
type Scheduler struct {
	cfg      Config
	cfgMu    sync.RWMutex
	queue    *TaskQueue
	results  *ResultStore
	resolver InstanceResolver
	client   *http.Client
	metrics  *Metrics

	// tracks all live tasks (queued + in-flight) for lookup by ID.
	live   map[string]*Task
	liveMu sync.RWMutex

	// webhookSem bounds the number of concurrent webhook delivery goroutines.
	webhookSem chan struct{}

	// cancellation
	cancels   map[string]context.CancelFunc
	cancelsMu sync.Mutex

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// New creates a scheduler with the given config and instance resolver.
func New(cfg Config, resolver InstanceResolver) *Scheduler {
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = 1000
	}
	if cfg.MaxPerAgent <= 0 {
		cfg.MaxPerAgent = 100
	}
	if cfg.MaxInflight <= 0 {
		cfg.MaxInflight = 20
	}
	if cfg.MaxPerAgentFlight <= 0 {
		cfg.MaxPerAgentFlight = 10
	}
	if cfg.ResultTTL <= 0 {
		cfg.ResultTTL = 5 * time.Minute
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 50
	}
	if cfg.WatcherInterval <= 0 {
		cfg.WatcherInterval = 30 * time.Second
	}

	return &Scheduler{
		cfg:        cfg,
		queue:      NewTaskQueue(cfg.MaxQueueSize, cfg.MaxPerAgent),
		results:    NewResultStore(cfg.ResultTTL),
		resolver:   resolver,
		client:     &http.Client{Timeout: 60 * time.Second},
		metrics:    newMetrics(),
		live:       make(map[string]*Task),
		cancels:    make(map[string]context.CancelFunc),
		stopCh:     make(chan struct{}),
		webhookSem: make(chan struct{}, 16),
	}
}

// Start launches workers and the deadline reaper.
func (s *Scheduler) Start() {
	s.results.StartReaper(10 * time.Second)

	for i := range s.cfg.WorkerCount {
		s.wg.Add(1)
		go s.worker(i)
	}

	s.wg.Add(1)
	go s.deadlineReaper()

	slog.Info("scheduler started", "workers", s.cfg.WorkerCount, "strategy", s.cfg.Strategy)
}

// Stop gracefully shuts down the scheduler. Queued tasks are cancelled.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		slog.Info("scheduler stopping")
		close(s.stopCh)
		s.wg.Wait()
		s.results.Stop()

		s.liveMu.Lock()
		for id, t := range s.live {
			if !t.GetState().IsTerminal() {
				_ = t.SetState(StateCancelled)
				t.Error = "scheduler shutdown"
				s.results.Store(t)
			}
			delete(s.live, id)
		}
		s.liveMu.Unlock()

		slog.Info("scheduler stopped")
	})
}

// Submit creates a new task from the request and enqueues it.
func (s *Scheduler) Submit(req SubmitRequest) (*Task, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}

	now := timeNow()
	deadline := now.Add(60 * time.Second)
	if req.Deadline != "" {
		parsed, err := time.Parse(time.RFC3339, req.Deadline)
		if err != nil {
			return nil, fmt.Errorf("invalid deadline format: %w", err)
		}
		if parsed.Before(now) {
			return nil, fmt.Errorf("deadline is in the past")
		}
		deadline = parsed
	}

	t := &Task{
		ID:          generateTaskID(),
		AgentID:     req.AgentID,
		Action:      req.Action,
		TabID:       req.TabID,
		Ref:         req.Ref,
		Params:      req.Params,
		Priority:    req.Priority,
		State:       StateQueued,
		Deadline:    deadline,
		CreatedAt:   now,
		CallbackURL: req.CallbackURL,
	}

	pos, err := s.queue.Enqueue(t)
	if err != nil {
		t.State = StateRejected
		t.Error = err.Error()
		s.results.Store(t)
		s.metrics.recordReject(req.AgentID)
		slog.Warn("task rejected", "task", t.ID, "agent", req.AgentID, "err", err)
		return t, fmt.Errorf("rejected: %w", err)
	}

	t.Position = pos

	s.liveMu.Lock()
	s.live[t.ID] = t
	s.liveMu.Unlock()

	s.results.Store(t)
	s.metrics.recordSubmit(req.AgentID)
	slog.Info("task submitted", "task", t.ID, "agent", req.AgentID, "action", t.Action, "priority", t.Priority, "position", pos)
	return t, nil
}

// GetTask retrieves a task by ID from live or completed results.
func (s *Scheduler) GetTask(taskID string) *Task {
	s.liveMu.RLock()
	if t, ok := s.live[taskID]; ok {
		s.liveMu.RUnlock()
		return t
	}
	s.liveMu.RUnlock()
	return s.results.Get(taskID)
}

// Cancel attempts to cancel a task.
func (s *Scheduler) Cancel(taskID string) error {
	s.liveMu.RLock()
	t, ok := s.live[taskID]
	s.liveMu.RUnlock()

	if !ok {
		stored := s.results.Get(taskID)
		if stored == nil {
			return fmt.Errorf("task %q not found", taskID)
		}
		if stored.State.IsTerminal() {
			return fmt.Errorf("task %q already in terminal state %q", taskID, stored.State)
		}
		return fmt.Errorf("task %q not found in active set", taskID)
	}

	state := t.GetState()
	if state.IsTerminal() {
		return fmt.Errorf("task %q already in terminal state %q", taskID, state)
	}

	if state == StateQueued {
		s.queue.Remove(t.ID, t.AgentID)
	}

	s.cancelsMu.Lock()
	if cancel, exists := s.cancels[taskID]; exists {
		cancel()
	}
	s.cancelsMu.Unlock()

	if err := t.SetState(StateCancelled); err != nil {
		return fmt.Errorf("cancel failed: %w", err)
	}

	s.metrics.recordCancel(t.AgentID)
	slog.Info("task cancelled", "task", t.ID, "agent", t.AgentID, "previousState", state)

	s.finishTask(t)
	return nil
}

// ListTasks returns tasks matching the given filters.
func (s *Scheduler) ListTasks(agentID string, states []TaskState) []*Task {
	return s.results.List(agentID, states)
}

// QueueStats returns current queue metrics.
func (s *Scheduler) QueueStats() QueueStats {
	return s.queue.Stats()
}

// GetMetrics returns a snapshot of all scheduler metrics.
func (s *Scheduler) GetMetrics() MetricsSnapshot {
	return s.metrics.Snapshot()
}

func (s *Scheduler) inflightLimits() (perAgent, global int) {
	s.cfgMu.RLock()
	perAgent = s.cfg.MaxPerAgentFlight
	global = s.cfg.MaxInflight
	s.cfgMu.RUnlock()
	return
}

func (s *Scheduler) worker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		task := s.queue.Dequeue(s.inflightLimits())
		if task == nil {
			select {
			case <-s.stopCh:
				return
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}

		s.dispatch(task)
	}
}

func (s *Scheduler) dispatch(t *Task) {
	dispatchStart := timeNow()

	if err := t.SetState(StateAssigned); err != nil {
		slog.Warn("task state transition failed", "task", t.ID, "err", err)
		s.finishTask(t)
		return
	}
	slog.Info("task assigned", "task", t.ID, "agent", t.AgentID, "action", t.Action)
	s.results.Store(t)

	ctx, cancel := context.WithDeadline(context.Background(), t.Deadline)

	s.cancelsMu.Lock()
	s.cancels[t.ID] = cancel
	s.cancelsMu.Unlock()

	defer func() {
		cancel()
		s.cancelsMu.Lock()
		delete(s.cancels, t.ID)
		s.cancelsMu.Unlock()
	}()

	if err := t.SetState(StateRunning); err != nil {
		slog.Warn("task state transition failed", "task", t.ID, "err", err)
		s.finishTask(t)
		return
	}
	slog.Info("task running", "task", t.ID, "agent", t.AgentID)
	s.results.Store(t)

	result, execErr := s.executeTask(ctx, t)

	latency := timeNow().Sub(dispatchStart)
	s.metrics.recordDispatchLatency(latency)

	if execErr != nil {
		t.Error = execErr.Error()
		if stateErr := t.SetState(StateFailed); stateErr != nil {
			slog.Warn("failed to mark task as failed", "task", t.ID, "err", stateErr)
		}
		s.metrics.recordFail(t.AgentID)
		slog.Info("task failed", "task", t.ID, "agent", t.AgentID, "err", execErr, "latencyMs", latency.Milliseconds())
	} else {
		t.Result = result
		if stateErr := t.SetState(StateDone); stateErr != nil {
			slog.Warn("failed to mark task as done", "task", t.ID, "err", stateErr)
		}
		s.metrics.recordComplete(t.AgentID)
		slog.Info("task completed", "task", t.ID, "agent", t.AgentID, "action", t.Action, "latencyMs", latency.Milliseconds())
	}

	s.finishTask(t)
}

func (s *Scheduler) executeTask(ctx context.Context, t *Task) (any, error) {
	if t.TabID == "" {
		return nil, fmt.Errorf("tabId is required for task execution")
	}

	port, err := s.resolver.ResolveTabInstance(t.TabID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve tab %q: %w", t.TabID, err)
	}

	// Build the request body matching the immediate-path action format.
	body := map[string]any{
		"kind": t.Action,
	}
	if t.Ref != "" {
		body["ref"] = t.Ref
	}
	for k, v := range t.Params {
		body[k] = v
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode task body: %w", err)
	}

	targetURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("localhost", port),
		Path:   fmt.Sprintf("/tabs/%s/action", t.TabID),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(activity.HeaderPTSource, "scheduler")
	req.Header.Set(activity.HeaderPTTabID, t.TabID)
	if t.AgentID != "" {
		req.Header.Set(activity.HeaderAgentID, t.AgentID)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executor request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read executor response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("executor returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), nil
	}
	return result, nil
}

func (s *Scheduler) finishTask(t *Task) {
	s.results.Store(t)
	s.queue.Complete(t.AgentID)

	s.liveMu.Lock()
	delete(s.live, t.ID)
	s.liveMu.Unlock()

	// Fire webhook asynchronously if configured.
	if t.CallbackURL != "" && t.GetState().IsTerminal() {
		select {
		case s.webhookSem <- struct{}{}:
			go func() {
				defer func() { <-s.webhookSem }()
				sendWebhook(t.CallbackURL, t)
			}()
		default:
			slog.Warn("webhook: too many in-flight deliveries, dropping", "task", t.ID)
		}
	}
}

func (s *Scheduler) deadlineReaper() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			expired := s.queue.ExpireDeadlined()
			for _, t := range expired {
				t.Error = "deadline exceeded while queued"
				if err := t.SetState(StateFailed); err != nil {
					slog.Warn("deadline reaper state transition failed", "task", t.ID, "err", err)
				}
				s.metrics.recordExpire()
				s.metrics.recordFail(t.AgentID)
				slog.Info("task expired", "task", t.ID, "agent", t.AgentID)
				s.finishTask(t)
			}
		}
	}
}
