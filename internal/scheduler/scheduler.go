package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
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

// DefaultConfig returns safe defaults. The knobs an operator can set live in
// internal/config, which owns their defaults and reports them through `config get`;
// only the one this package does not expose as a config key is written here.
func DefaultConfig() Config {
	out := ConfigFromRuntime(config.DefaultSchedulerConfig())
	out.WatcherInterval = 30 * time.Second
	return out
}

// ConfigFromRuntime maps the operator-facing scheduler settings onto this package's
// Config. It is the one place that conversion happens, so the seconds-to-duration
// step cannot drift between callers.
func ConfigFromRuntime(s config.SchedulerConfig) Config {
	return Config{
		Enabled:           s.Enabled,
		Strategy:          s.Strategy,
		MaxQueueSize:      s.MaxQueueSize,
		MaxPerAgent:       s.MaxPerAgent,
		MaxInflight:       s.MaxInflight,
		MaxPerAgentFlight: s.MaxPerAgentFlight,
		ResultTTL:         time.Duration(s.ResultTTLSec) * time.Second,
		WorkerCount:       s.WorkerCount,
		MaxBatchSize:      s.MaxBatchSize,
	}
}

// withDefaults fills every knob left at or below zero. A configured zero has always
// meant "use the default" here, never unlimited.
func withDefaults(cfg *Config) {
	defaults := DefaultConfig()
	if cfg.Strategy == "" {
		cfg.Strategy = defaults.Strategy
	}
	for _, knob := range []struct {
		value    *int
		fallback int
	}{
		{&cfg.MaxQueueSize, defaults.MaxQueueSize},
		{&cfg.MaxPerAgent, defaults.MaxPerAgent},
		{&cfg.MaxInflight, defaults.MaxInflight},
		{&cfg.MaxPerAgentFlight, defaults.MaxPerAgentFlight},
		{&cfg.WorkerCount, defaults.WorkerCount},
		{&cfg.MaxBatchSize, defaults.MaxBatchSize},
	} {
		if *knob.value <= 0 {
			*knob.value = knob.fallback
		}
	}
	if cfg.ResultTTL <= 0 {
		cfg.ResultTTL = defaults.ResultTTL
	}
	if cfg.WatcherInterval <= 0 {
		cfg.WatcherInterval = defaults.WatcherInterval
	}
}

// Scheduler is the core dispatch engine.
type Scheduler struct {
	cfg      Config
	cfgMu    sync.RWMutex
	queue    *TaskQueue
	results  *ResultStore
	executor TaskExecutor
	metrics  *Metrics

	// tracks all live tasks (queued + in-flight) for lookup by ID.
	live   map[string]*Task
	liveMu sync.RWMutex

	webhooks *webhookDispatcher

	cancels   map[string]context.CancelFunc
	cancelsMu sync.Mutex

	startOnce   sync.Once
	stopOnce    sync.Once
	stopCh      chan struct{}
	wg          sync.WaitGroup
	noAutoStart bool // testing only: suppress ensureRunning from Submit
}

// New creates a scheduler with the given config and instance resolver.
func New(cfg Config, resolver InstanceResolver) *Scheduler {
	withDefaults(&cfg)

	return &Scheduler{
		cfg:      cfg,
		queue:    NewTaskQueue(cfg.MaxQueueSize, cfg.MaxPerAgent),
		results:  NewResultStore(cfg.ResultTTL),
		executor: &actionEndpointExecutor{resolver: resolver, client: &http.Client{Timeout: 60 * time.Second}},
		metrics:  newMetrics(),
		live:     make(map[string]*Task),
		cancels:  make(map[string]context.CancelFunc),
		stopCh:   make(chan struct{}),
		webhooks: newWebhookDispatcher(16),
	}
}

// Start launches workers and the deadline reaper. If Start is not called,
// workers are launched lazily on the first Submit to avoid idle CPU cost.
func (s *Scheduler) Start() {
	s.ensureRunning()
}

func (s *Scheduler) ensureRunning() {
	s.startOnce.Do(func() {
		s.results.StartReaper(10 * time.Second)

		for i := range s.cfg.WorkerCount {
			s.wg.Add(1)
			go s.worker(i)
		}

		s.wg.Add(1)
		go s.deadlineReaper()

		slog.Info("scheduler started", "workers", s.cfg.WorkerCount)
	})
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
	if !s.noAutoStart {
		s.ensureRunning()
	}

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
		Selector:    req.Selector,
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
		// Route through SetState so the reject is stamped (CompletedAt) like
		// other terminal states; the Queued→Rejected transition is always valid
		// here, so the error is intentionally ignored.
		_ = t.SetState(StateRejected)
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
		task := s.queue.Dequeue(s.inflightLimits())
		if task == nil {
			select {
			case <-s.stopCh:
				return
			case <-s.queue.Ready():
				continue
			}
		}

		s.dispatch(task)
	}
}

func (s *Scheduler) dispatch(t *Task) {
	// The in-flight slot belongs to the dequeue that produced this task, so it is
	// released here and nowhere else: finishTask also runs for tasks that expired
	// or were cancelled while QUEUED, which never took a slot, and for a running
	// task that was cancelled, whose slot this dispatch is still holding. Releasing
	// there decremented another task's count, and the count is what bounds both
	// MaxPerAgentFlight and MaxInflight. Deferred so every exit path — including a
	// panic in an executor — gives the slot back.
	defer s.queue.Complete(t.AgentID)

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

	result, execErr := s.executor.Execute(ctx, t)

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

// finishTask publishes a task's terminal state. It does NOT release an in-flight
// slot: it is reached from the deadline reaper and from Cancel for tasks that were
// still queued, and dispatch owns the slot for the ones that were not.
func (s *Scheduler) finishTask(t *Task) {
	s.results.Store(t)

	s.liveMu.Lock()
	delete(s.live, t.ID)
	s.liveMu.Unlock()

	s.webhooks.fire(t)
}

func (s *Scheduler) deadlineReaper() {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
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
