package scheduler

import (
	"testing"
	"time"
)

func TestQueueEnqueueDequeue(t *testing.T) {
	q := NewTaskQueue(10, 5)

	t1 := &Task{ID: "t1", AgentID: "a1", Priority: 5, CreatedAt: time.Now()}
	t2 := &Task{ID: "t2", AgentID: "a1", Priority: 1, CreatedAt: time.Now()}

	if _, err := q.Enqueue(t1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(t2); err != nil {
		t.Fatal(err)
	}

	// t2 has higher priority (lower number), should come first.
	got := q.Dequeue(10, 20)
	if got == nil {
		t.Fatal("expected a task")
	}
	if got.ID != "t2" {
		t.Errorf("expected t2 (priority 1), got %s (priority %d)", got.ID, got.Priority)
	}

	got2 := q.Dequeue(10, 20)
	if got2 == nil || got2.ID != "t1" {
		t.Error("expected t1 next")
	}
}

func TestQueueGlobalLimit(t *testing.T) {
	q := NewTaskQueue(2, 10)

	q.Enqueue(&Task{ID: "t1", AgentID: "a1", CreatedAt: time.Now()})
	q.Enqueue(&Task{ID: "t2", AgentID: "a1", CreatedAt: time.Now()})
	_, err := q.Enqueue(&Task{ID: "t3", AgentID: "a1", CreatedAt: time.Now()})
	if err == nil {
		t.Error("should reject when global limit reached")
	}
}

func TestQueuePerAgentLimit(t *testing.T) {
	q := NewTaskQueue(100, 2)

	q.Enqueue(&Task{ID: "t1", AgentID: "a1", CreatedAt: time.Now()})
	q.Enqueue(&Task{ID: "t2", AgentID: "a1", CreatedAt: time.Now()})
	_, err := q.Enqueue(&Task{ID: "t3", AgentID: "a1", CreatedAt: time.Now()})
	if err == nil {
		t.Error("should reject when per-agent limit reached")
	}

	// Different agent should still work.
	if _, err := q.Enqueue(&Task{ID: "t4", AgentID: "a2", CreatedAt: time.Now()}); err != nil {
		t.Errorf("different agent should succeed: %v", err)
	}
}

func TestQueueFairness(t *testing.T) {
	q := NewTaskQueue(100, 100)

	// Agent a1 has 3 tasks, a2 has 1 task.
	for i := range 3 {
		q.Enqueue(&Task{ID: "a1-" + string(rune('0'+i)), AgentID: "a1", CreatedAt: time.Now()})
	}
	q.Enqueue(&Task{ID: "a2-0", AgentID: "a2", CreatedAt: time.Now()})

	// First dequeue: both have 0 inflight, either could be picked.
	got1 := q.Dequeue(10, 20)
	if got1 == nil {
		t.Fatal("expected a task")
	}

	// Now one agent has 1 inflight. The agent with 0 should be preferred.
	got2 := q.Dequeue(10, 20)
	if got2 == nil {
		t.Fatal("expected a task")
	}

	// The two dequeued tasks should be from different agents (fairness).
	if got1.AgentID == got2.AgentID {
		t.Errorf("fairness: expected different agents, got %s and %s", got1.AgentID, got2.AgentID)
	}
}

func TestQueueInflightLimit(t *testing.T) {
	q := NewTaskQueue(100, 100)

	q.Enqueue(&Task{ID: "t1", AgentID: "a1", CreatedAt: time.Now()})
	q.Enqueue(&Task{ID: "t2", AgentID: "a1", CreatedAt: time.Now()})

	// Dequeue with max inflight = 1
	got := q.Dequeue(1, 2)
	if got == nil {
		t.Fatal("first dequeue should work")
	}

	// Second dequeue should be blocked (per-agent limit).
	got2 := q.Dequeue(1, 2)
	if got2 != nil {
		t.Error("should be blocked by per-agent inflight limit")
	}

	// Complete the first, now second should work.
	q.Complete("a1")
	got3 := q.Dequeue(1, 2)
	if got3 == nil {
		t.Error("should dequeue after completing")
	}
}

func TestQueueRemove(t *testing.T) {
	q := NewTaskQueue(10, 10)
	q.Enqueue(&Task{ID: "t1", AgentID: "a1", CreatedAt: time.Now()})
	q.Enqueue(&Task{ID: "t2", AgentID: "a1", CreatedAt: time.Now()})

	if !q.Remove("t1", "a1") {
		t.Error("should find and remove t1")
	}
	if q.Remove("t1", "a1") {
		t.Error("removing again should return false")
	}

	// Only t2 should remain.
	got := q.Dequeue(10, 20)
	if got == nil || got.ID != "t2" {
		t.Error("expected t2 remaining")
	}
}

func TestQueueExpireDeadlined(t *testing.T) {
	old := timeNow
	defer func() { timeNow = old }()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return now }

	q := NewTaskQueue(10, 10)

	pastDeadline := &Task{
		ID: "expired", AgentID: "a1", CreatedAt: now,
		Deadline: now.Add(-1 * time.Second),
	}
	futureDeadline := &Task{
		ID: "valid", AgentID: "a1", CreatedAt: now,
		Deadline: now.Add(1 * time.Hour),
	}
	q.Enqueue(pastDeadline)
	q.Enqueue(futureDeadline)

	expired := q.ExpireDeadlined()
	if len(expired) != 1 || expired[0].ID != "expired" {
		t.Errorf("expected 1 expired task, got %d", len(expired))
	}

	stats := q.Stats()
	if stats.TotalQueued != 1 {
		t.Errorf("expected 1 remaining, got %d", stats.TotalQueued)
	}
}

func TestQueueStats(t *testing.T) {
	q := NewTaskQueue(100, 100)
	q.Enqueue(&Task{ID: "t1", AgentID: "a1", CreatedAt: time.Now()})
	q.Enqueue(&Task{ID: "t2", AgentID: "a2", CreatedAt: time.Now()})
	q.Dequeue(10, 20) // a1 or a2 gets 1 inflight

	stats := q.Stats()
	if stats.TotalQueued != 1 {
		t.Errorf("expected 1 queued, got %d", stats.TotalQueued)
	}
	if stats.TotalInflight != 1 {
		t.Errorf("expected 1 inflight, got %d", stats.TotalInflight)
	}
	if len(stats.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(stats.Agents))
	}
}
