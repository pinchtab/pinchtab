package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockResolver struct {
	port string
	err  error
}

func (m *mockResolver) ResolveTabInstance(tabID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.port, nil
}

func newTestScheduler(t *testing.T) (*Scheduler, *httptest.Server) {
	t.Helper()

	// Mock executor that returns success.
	executor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))

	// Extract port from executor URL.
	parts := strings.Split(executor.URL, ":")
	port := parts[len(parts)-1]

	cfg := DefaultConfig()
	cfg.WorkerCount = 2
	cfg.MaxInflight = 5
	cfg.MaxPerAgentFlight = 3

	resolver := &mockResolver{port: port}
	s := New(cfg, resolver)

	return s, executor
}

func TestSchedulerSubmitAndGet(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	task, err := s.Submit(SubmitRequest{
		AgentID: "agent-1",
		Action:  "click",
		TabID:   "tab-1",
		Ref:     "e14",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	if task.ID == "" {
		t.Error("task should have an ID")
	}
	if task.State != StateQueued {
		t.Errorf("expected queued, got %s", task.State)
	}

	got := s.GetTask(task.ID)
	if got == nil {
		t.Fatal("should find submitted task")
	}
}

func TestSchedulerSubmitValidation(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	_, err := s.Submit(SubmitRequest{Action: "click"})
	if err == nil {
		t.Error("should reject missing agentId")
	}

	_, err = s.Submit(SubmitRequest{AgentID: "a1"})
	if err == nil {
		t.Error("should reject missing action")
	}
}

func TestSchedulerSubmitDeadlineInPast(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	_, err := s.Submit(SubmitRequest{
		AgentID:  "a1",
		Action:   "click",
		Deadline: "2020-01-01T00:00:00Z",
	})
	if err == nil {
		t.Error("should reject past deadline")
	}
}

func TestSchedulerSubmitBadDeadlineFormat(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	_, err := s.Submit(SubmitRequest{
		AgentID:  "a1",
		Action:   "click",
		Deadline: "not-a-date",
	})
	if err == nil {
		t.Error("should reject bad deadline format")
	}
}

func TestSchedulerCancel_Queued(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	task, _ := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})

	if err := s.Cancel(task.ID); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	got := s.GetTask(task.ID)
	if got == nil {
		t.Fatal("should still find task in results")
	}
	if got.GetState() != StateCancelled {
		t.Errorf("expected cancelled, got %s", got.GetState())
	}
}

func TestSchedulerCancel_NotFound(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	if err := s.Cancel("nonexistent"); err == nil {
		t.Error("should error on unknown task ID")
	}
}

func TestSchedulerListTasks(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-1"})
	s.Submit(SubmitRequest{AgentID: "a2", Action: "navigate", TabID: "tab-2"})

	all := s.ListTasks("", nil)
	if len(all) < 2 {
		t.Errorf("expected at least 2 tasks, got %d", len(all))
	}

	a1Only := s.ListTasks("a1", nil)
	if len(a1Only) != 1 {
		t.Errorf("expected 1 task for a1, got %d", len(a1Only))
	}
}

func TestSchedulerDispatchAndComplete(t *testing.T) {
	executor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer executor.Close()

	parts := strings.Split(executor.URL, ":")
	port := parts[len(parts)-1]

	cfg := DefaultConfig()
	cfg.WorkerCount = 1
	cfg.MaxInflight = 5
	cfg.MaxPerAgentFlight = 5

	resolver := &mockResolver{port: port}
	s := New(cfg, resolver)
	s.Start()
	defer s.Stop()

	task, err := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
		Ref:     "e14",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Wait for task to complete.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("task did not complete in time, state: %s", s.GetTask(task.ID).GetState())
		default:
		}

		got := s.GetTask(task.ID)
		if got != nil && got.GetState() == StateDone {
			if got.Result == nil {
				t.Error("completed task should have a result")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSchedulerDispatchFailure(t *testing.T) {
	executor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"internal"}`)
	}))
	defer executor.Close()

	parts := strings.Split(executor.URL, ":")
	port := parts[len(parts)-1]

	cfg := DefaultConfig()
	cfg.WorkerCount = 1

	s := New(cfg, &mockResolver{port: port})
	s.Start()
	defer s.Stop()

	task, _ := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("task did not fail in time, state: %s", s.GetTask(task.ID).GetState())
		default:
		}

		got := s.GetTask(task.ID)
		if got != nil && got.GetState() == StateFailed {
			if got.Error == "" {
				t.Error("failed task should have an error message")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSchedulerResolverError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 1

	s := New(cfg, &mockResolver{err: fmt.Errorf("no instance")})
	s.Start()
	defer s.Stop()

	task, _ := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("task did not fail in time")
		default:
		}

		got := s.GetTask(task.ID)
		if got != nil && got.GetState() == StateFailed {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSchedulerQueueStats(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-1"})
	s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-2"})

	stats := s.QueueStats()
	if stats.TotalQueued != 2 {
		t.Errorf("expected 2 queued, got %d", stats.TotalQueued)
	}
}

func TestSchedulerStopCancelsQueued(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	task, _ := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})

	s.Stop()

	got := s.GetTask(task.ID)
	if got != nil && got.GetState() != StateCancelled {
		t.Errorf("expected cancelled after stop, got %s", got.GetState())
	}
}
