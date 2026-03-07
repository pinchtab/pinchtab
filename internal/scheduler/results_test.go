package scheduler

import (
	"testing"
	"time"
)

func TestResultStoreStoreAndGet(t *testing.T) {
	rs := NewResultStore(5 * time.Minute)

	task := &Task{ID: "tsk_1", AgentID: "a1", State: StateDone}
	rs.Store(task)

	got := rs.Get("tsk_1")
	if got == nil {
		t.Fatal("expected to find stored task")
	}
	if got.ID != "tsk_1" {
		t.Errorf("expected tsk_1, got %s", got.ID)
	}
}

func TestResultStoreGetMissing(t *testing.T) {
	rs := NewResultStore(5 * time.Minute)
	if rs.Get("nonexistent") != nil {
		t.Error("should return nil for missing")
	}
}

func TestResultStoreList(t *testing.T) {
	rs := NewResultStore(5 * time.Minute)

	rs.Store(&Task{ID: "t1", AgentID: "a1", State: StateDone})
	rs.Store(&Task{ID: "t2", AgentID: "a1", State: StateQueued})
	rs.Store(&Task{ID: "t3", AgentID: "a2", State: StateDone})

	// All tasks
	all := rs.List("", nil)
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}

	// Filter by agent
	a1Only := rs.List("a1", nil)
	if len(a1Only) != 2 {
		t.Errorf("expected 2 for a1, got %d", len(a1Only))
	}

	// Filter by state
	doneOnly := rs.List("", []TaskState{StateDone})
	if len(doneOnly) != 2 {
		t.Errorf("expected 2 done, got %d", len(doneOnly))
	}

	// Filter by both
	a1Done := rs.List("a1", []TaskState{StateDone})
	if len(a1Done) != 1 {
		t.Errorf("expected 1, got %d", len(a1Done))
	}
}

func TestResultStoreEviction(t *testing.T) {
	old := timeNow
	defer func() { timeNow = old }()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return now }

	rs := NewResultStore(5 * time.Minute)

	rs.Store(&Task{
		ID: "old", State: StateDone,
		CompletedAt: now.Add(-10 * time.Minute),
	})
	rs.Store(&Task{
		ID: "recent", State: StateDone,
		CompletedAt: now.Add(-1 * time.Minute),
	})
	rs.Store(&Task{
		ID: "active", State: StateRunning,
	})

	rs.evict()

	if rs.Get("old") != nil {
		t.Error("old task should be evicted")
	}
	if rs.Get("recent") == nil {
		t.Error("recent task should not be evicted")
	}
	if rs.Get("active") == nil {
		t.Error("active task should not be evicted")
	}
}

func TestResultStoreDelete(t *testing.T) {
	rs := NewResultStore(5 * time.Minute)
	rs.Store(&Task{ID: "t1", State: StateDone})
	rs.Delete("t1")
	if rs.Get("t1") != nil {
		t.Error("should be deleted")
	}
}
