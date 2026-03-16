package scheduler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// TaskState represents the current state of a task.
type TaskState string

const (
	StateQueued    TaskState = "queued"
	StateAssigned  TaskState = "assigned"
	StateRunning   TaskState = "running"
	StateDone      TaskState = "done"
	StateFailed    TaskState = "failed"
	StateCancelled TaskState = "cancelled"
	StateRejected  TaskState = "rejected"
)

// IsTerminal returns true for states that will not change.
func (s TaskState) IsTerminal() bool {
	switch s {
	case StateDone, StateFailed, StateCancelled, StateRejected:
		return true
	}
	return false
}

// Task represents a scheduled unit of work dispatched to the executor.
type Task struct {
	mu sync.RWMutex

	ID       string         `json:"taskId"`
	AgentID  string         `json:"agentId"`
	Action   string         `json:"action"`
	TabID    string         `json:"tabId,omitempty"`
	Selector string         `json:"selector,omitempty"`
	Ref      string         `json:"ref,omitempty"` // deprecated: use Selector
	Params   map[string]any `json:"params,omitempty"`
	Priority int            `json:"priority"`
	State    TaskState      `json:"state"`
	Deadline time.Time      `json:"deadline,omitempty"`

	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	LatencyMs   int64     `json:"latencyMs,omitempty"`

	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`

	// CallbackURL receives a POST with the task snapshot on completion.
	CallbackURL string `json:"callbackUrl,omitempty"`

	// position is the queue position at submission time.
	Position int `json:"position,omitempty"`
}

// SetState transitions the task to the given state. Returns an error if
// the transition is invalid (e.g. terminal → anything).
func (t *Task) SetState(next TaskState) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.State.IsTerminal() {
		return fmt.Errorf("cannot transition from terminal state %q to %q", t.State, next)
	}

	switch {
	case t.State == StateQueued && (next == StateAssigned || next == StateCancelled || next == StateFailed || next == StateRejected):
	case t.State == StateAssigned && (next == StateRunning || next == StateCancelled):
	case t.State == StateRunning && (next == StateDone || next == StateFailed || next == StateCancelled):
	default:
		return fmt.Errorf("invalid state transition: %q → %q", t.State, next)
	}

	now := time.Now()
	t.State = next

	switch next {
	case StateAssigned:
		t.StartedAt = now
	case StateRunning:
		if t.StartedAt.IsZero() {
			t.StartedAt = now
		}
	case StateDone, StateFailed, StateCancelled:
		t.CompletedAt = now
		if !t.StartedAt.IsZero() {
			t.LatencyMs = now.Sub(t.StartedAt).Milliseconds()
		}
	}
	return nil
}

// GetState returns the current task state.
func (t *Task) GetState() TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.State
}

// Snapshot returns a read-consistent copy of the task for serialization.
func (t *Task) Snapshot() *Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return &Task{
		ID:          t.ID,
		AgentID:     t.AgentID,
		Action:      t.Action,
		TabID:       t.TabID,
		Selector:    t.Selector,
		Ref:         t.Ref,
		Params:      t.Params,
		Priority:    t.Priority,
		State:       t.State,
		Deadline:    t.Deadline,
		CreatedAt:   t.CreatedAt,
		StartedAt:   t.StartedAt,
		CompletedAt: t.CompletedAt,
		LatencyMs:   t.LatencyMs,
		Result:      t.Result,
		Error:       t.Error,
		CallbackURL: t.CallbackURL,
		Position:    t.Position,
	}
}

// SubmitRequest is the JSON body for POST /tasks.
type SubmitRequest struct {
	AgentID     string         `json:"agentId"`
	Action      string         `json:"action"`
	TabID       string         `json:"tabId,omitempty"`
	Selector    string         `json:"selector,omitempty"`
	Ref         string         `json:"ref,omitempty"` // deprecated: use Selector
	Params      map[string]any `json:"params,omitempty"`
	Priority    int            `json:"priority,omitempty"`
	Deadline    string         `json:"deadline,omitempty"`
	CallbackURL string         `json:"callbackUrl,omitempty"`
}

// Validate checks that the request has the minimum required fields.
func (r *SubmitRequest) Validate() error {
	if r.AgentID == "" {
		return fmt.Errorf("missing required field 'agentId'")
	}
	if r.Action == "" {
		return fmt.Errorf("missing required field 'action'")
	}
	return nil
}

// generateTaskID produces a random task ID in the format tsk_XXXXXXXX.
func generateTaskID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto/rand fails.
		return fmt.Sprintf("tsk_%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return "tsk_" + hex.EncodeToString(b)
}
