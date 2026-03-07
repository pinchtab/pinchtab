package scheduler

import (
	"sync"
	"time"
)

// ResultStore holds completed task results in memory with TTL-based expiry.
type ResultStore struct {
	mu      sync.RWMutex
	tasks   map[string]*Task
	ttl     time.Duration
	closeCh chan struct{}
}

// NewResultStore creates a store that evicts terminal tasks after ttl.
func NewResultStore(ttl time.Duration) *ResultStore {
	return &ResultStore{
		tasks:   make(map[string]*Task),
		ttl:     ttl,
		closeCh: make(chan struct{}),
	}
}

// StartReaper begins a background goroutine that periodically evicts expired
// results. Call Stop() to terminate the goroutine.
func (rs *ResultStore) StartReaper(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rs.evict()
			case <-rs.closeCh:
				return
			}
		}
	}()
}

// Stop terminates the reaper goroutine.
func (rs *ResultStore) Stop() {
	select {
	case <-rs.closeCh:
	default:
		close(rs.closeCh)
	}
}

// Store saves a task snapshot into the result store.
func (rs *ResultStore) Store(t *Task) {
	snap := t.Snapshot()
	rs.mu.Lock()
	rs.tasks[snap.ID] = &snap
	rs.mu.Unlock()
}

// Get returns the task by ID or nil if not found.
func (rs *ResultStore) Get(taskID string) *Task {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.tasks[taskID]
}

// List returns all stored tasks, optionally filtered.
func (rs *ResultStore) List(agentID string, states []TaskState) []Task {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	stateSet := make(map[TaskState]bool, len(states))
	for _, s := range states {
		stateSet[s] = true
	}

	var out []Task
	for _, t := range rs.tasks {
		if agentID != "" && t.AgentID != agentID {
			continue
		}
		if len(stateSet) > 0 && !stateSet[t.State] {
			continue
		}
		out = append(out, *t)
	}
	return out
}

// Delete removes a task from the store.
func (rs *ResultStore) Delete(taskID string) {
	rs.mu.Lock()
	delete(rs.tasks, taskID)
	rs.mu.Unlock()
}

func (rs *ResultStore) evict() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	cutoff := timeNow().Add(-rs.ttl)
	for id, t := range rs.tasks {
		if t.State.IsTerminal() && !t.CompletedAt.IsZero() && t.CompletedAt.Before(cutoff) {
			delete(rs.tasks, id)
		}
	}
}
