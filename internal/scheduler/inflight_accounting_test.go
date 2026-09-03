package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

// blockingExecutor holds every task inside Execute until release is closed, so a
// test can observe the queue while work is genuinely in flight.
type blockingExecutor struct {
	entered chan string
	exited  chan string
	release chan struct{}
	once    sync.Once
}

func newBlockingExecutor(capacity int) *blockingExecutor {
	return &blockingExecutor{
		entered: make(chan string, capacity),
		exited:  make(chan string, capacity),
		release: make(chan struct{}),
	}
}

func (b *blockingExecutor) Execute(ctx context.Context, t *Task) (any, error) {
	b.entered <- t.ID
	defer func() { b.exited <- t.ID }()
	select {
	case <-b.release:
		return map[string]bool{"ok": true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *blockingExecutor) releaseAll() { b.once.Do(func() { close(b.release) }) }

func newBlockingScheduler(t *testing.T, perAgent, global, workers int) (*Scheduler, *blockingExecutor) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.WorkerCount = workers
	cfg.MaxInflight = global
	cfg.MaxPerAgentFlight = perAgent
	s := New(cfg, &mockResolver{port: "9999"})
	exec := newBlockingExecutor(workers + 2)
	s.executor = exec
	t.Cleanup(func() {
		exec.releaseAll()
		s.Stop()
	})
	return s, exec
}

func waitForInflight(t *testing.T, exec *blockingExecutor, n int) {
	t.Helper()
	for range n {
		select {
		case <-exec.entered:
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %d tasks to reach the executor", n)
		}
	}
}

// The in-flight count bounds MaxPerAgentFlight and MaxInflight, and only a
// dequeue takes a slot. Cancelling a task that is still QUEUED never took one, so
// releasing on its behalf decremented the slot held by a task that is actually
// running — under-counting the agent, and letting the scheduler dispatch past the
// very limits the count exists to enforce.
func TestCancellingAQueuedTaskLeavesTheRunningCountAlone(t *testing.T) {
	s, exec := newBlockingScheduler(t, 1, 1, 1)
	s.Start()

	running, err := s.Submit(SubmitRequest{AgentID: "agent-1", Action: "click", TabID: "tab-1", Ref: "e1"})
	if err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	waitForInflight(t, exec, 1)

	queued, err := s.Submit(SubmitRequest{AgentID: "agent-1", Action: "click", TabID: "tab-1", Ref: "e2"})
	if err != nil {
		t.Fatalf("submit queued task: %v", err)
	}
	if err := s.Cancel(queued.ID); err != nil {
		t.Fatalf("cancel queued task: %v", err)
	}

	stats := s.QueueStats()
	if stats.TotalInflight != 1 {
		t.Errorf("TotalInflight = %d after cancelling a queued task, want 1; task %s is still running", stats.TotalInflight, running.ID)
	}
	if got := stats.Agents["agent-1"].Inflight; got != 1 {
		t.Errorf("agent-1 inflight = %d, want 1; the agent is over its limit by the amount this under-counts", got)
	}
}

// A running task's slot is held by its dispatch. Cancelling it must not release
// the slot a second time — dispatch releases it when Execute returns — or the
// count drops below the work actually in flight.
func TestCancellingARunningTaskReleasesItsSlotOnlyOnce(t *testing.T) {
	s, exec := newBlockingScheduler(t, 2, 2, 2)
	s.Start()

	first, err := s.Submit(SubmitRequest{AgentID: "agent-1", Action: "click", TabID: "tab-1", Ref: "e1"})
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	if _, err := s.Submit(SubmitRequest{AgentID: "agent-1", Action: "click", TabID: "tab-1", Ref: "e2"}); err != nil {
		t.Fatalf("submit second: %v", err)
	}
	waitForInflight(t, exec, 2)

	if err := s.Cancel(first.ID); err != nil {
		t.Fatalf("cancel running task: %v", err)
	}

	// Wait for the cancelled task to leave the executor, so the only release still
	// owed is its own. Then require the count to SETTLE at one rather than merely
	// pass through it: a second release lands microseconds later, so an assertion
	// that stops at the first sight of the right number reports success on the way
	// down and pins nothing.
	select {
	case id := <-exec.exited:
		if id != first.ID {
			t.Fatalf("task %s left the executor first, want the cancelled task %s", id, first.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the cancelled task never returned from the executor")
	}

	// Reach one, then HOLD it. Reaching it only means the cancelled task's own
	// release has landed; an assertion that stops there reports success on the way
	// down, because a second release for the same task arrives microseconds later.
	reach := time.Now().Add(3 * time.Second)
	for s.QueueStats().TotalInflight != 1 {
		if time.Now().After(reach) {
			t.Fatalf("TotalInflight never reached 1 (last %d); the cancelled task's slot was not released", s.QueueStats().TotalInflight)
		}
		time.Sleep(5 * time.Millisecond)
	}
	hold := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(hold) {
		if got := s.QueueStats().TotalInflight; got != 1 {
			t.Fatalf("TotalInflight fell to %d while a task is still inside Execute; its slot was released twice", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
