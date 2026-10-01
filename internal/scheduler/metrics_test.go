package scheduler

import (
	"testing"
	"time"
)

func TestMetricsRecordSubmit(t *testing.T) {
	m := newMetrics()
	m.recordSubmit("a1")
	m.recordSubmit("a1")
	m.recordSubmit("a2")

	snap := m.Snapshot()
	if snap.TasksSubmitted != 3 {
		t.Errorf("expected 3 submitted, got %d", snap.TasksSubmitted)
	}
	if snap.Agents["a1"].Submitted != 2 {
		t.Errorf("expected a1 submitted=2, got %d", snap.Agents["a1"].Submitted)
	}
	if snap.Agents["a2"].Submitted != 1 {
		t.Errorf("expected a2 submitted=1, got %d", snap.Agents["a2"].Submitted)
	}
}

func TestMetricsRecordReject(t *testing.T) {
	m := newMetrics()
	m.recordReject("a1")

	snap := m.Snapshot()
	if snap.TasksRejected != 1 {
		t.Errorf("expected 1 rejected, got %d", snap.TasksRejected)
	}
	if snap.Agents["a1"].Rejected != 1 {
		t.Errorf("expected a1 rejected=1, got %d", snap.Agents["a1"].Rejected)
	}
}

func TestMetricsRecordComplete(t *testing.T) {
	m := newMetrics()
	m.recordComplete("a1")
	m.recordComplete("a1")

	snap := m.Snapshot()
	if snap.TasksCompleted != 2 {
		t.Errorf("expected 2 completed, got %d", snap.TasksCompleted)
	}
	if snap.Agents["a1"].Completed != 2 {
		t.Errorf("expected a1 completed=2, got %d", snap.Agents["a1"].Completed)
	}
}

func TestMetricsRecordFail(t *testing.T) {
	m := newMetrics()
	m.recordFail("a1")

	snap := m.Snapshot()
	if snap.TasksFailed != 1 {
		t.Errorf("expected 1 failed, got %d", snap.TasksFailed)
	}
	if snap.Agents["a1"].Failed != 1 {
		t.Errorf("expected a1 failed=1, got %d", snap.Agents["a1"].Failed)
	}
}

func TestMetricsRecordCancel(t *testing.T) {
	m := newMetrics()
	m.recordCancel("a1")

	snap := m.Snapshot()
	if snap.TasksCancelled != 1 {
		t.Errorf("expected 1 cancelled, got %d", snap.TasksCancelled)
	}
	if snap.Agents["a1"].Cancelled != 1 {
		t.Errorf("expected a1 cancelled=1, got %d", snap.Agents["a1"].Cancelled)
	}
}

func TestMetricsRecordExpire(t *testing.T) {
	m := newMetrics()
	m.recordExpire()
	m.recordExpire()

	snap := m.Snapshot()
	if snap.TasksExpired != 2 {
		t.Errorf("expected 2 expired, got %d", snap.TasksExpired)
	}
}

func TestMetricsDispatchLatency(t *testing.T) {
	m := newMetrics()
	m.recordDispatchLatency(100 * time.Millisecond)
	m.recordDispatchLatency(200 * time.Millisecond)

	snap := m.Snapshot()
	if snap.DispatchCount != 2 {
		t.Errorf("expected 2 dispatches, got %d", snap.DispatchCount)
	}
	// Average of 100ms and 200ms = 150ms.
	if snap.AvgDispatchLatency < 140 || snap.AvgDispatchLatency > 160 {
		t.Errorf("expected avg ~150ms, got %.1f", snap.AvgDispatchLatency)
	}
}

func TestMetricsSnapshotIsolation(t *testing.T) {
	m := newMetrics()
	m.recordSubmit("a1")
	snap1 := m.Snapshot()

	m.recordSubmit("a1")
	snap2 := m.Snapshot()

	if snap1.TasksSubmitted != 1 {
		t.Error("snapshot1 should not be affected by later records")
	}
	if snap2.TasksSubmitted != 2 {
		t.Error("snapshot2 should reflect both records")
	}
}

func TestMetricsConcurrentAgentCreation(t *testing.T) {
	m := newMetrics()
	done := make(chan struct{})

	for i := range 10 {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			agent := "agent"
			if id%2 == 0 {
				agent = "agent-even"
			}
			m.recordSubmit(agent)
		}(i)
	}

	for range 10 {
		<-done
	}

	snap := m.Snapshot()
	if snap.TasksSubmitted != 10 {
		t.Errorf("expected 10 submitted, got %d", snap.TasksSubmitted)
	}
}

func TestMetricsAvgLatencyZeroDispatches(t *testing.T) {
	m := newMetrics()
	snap := m.Snapshot()
	if snap.AvgDispatchLatency != 0 {
		t.Errorf("expected 0 avg latency with no dispatches, got %.1f", snap.AvgDispatchLatency)
	}
}

func TestMetricsWiredInSubmit(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	_, _ = s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})

	snap := s.GetMetrics()
	if snap.TasksSubmitted != 1 {
		t.Errorf("expected 1 submitted, got %d", snap.TasksSubmitted)
	}
	if snap.Agents["a1"].Submitted != 1 {
		t.Errorf("expected a1 submitted=1, got %d", snap.Agents["a1"].Submitted)
	}
}

func TestMetricsWiredInCancel(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	task, err := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if err := s.Cancel(task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	snap := s.GetMetrics()
	if snap.TasksCancelled != 1 {
		t.Errorf("expected 1 cancelled, got %d", snap.TasksCancelled)
	}
}

func TestMetricsWiredInDispatch(t *testing.T) {
	s, executor := newTestScheduler(t)
	defer executor.Close()

	s.Start()
	defer s.Stop()

	_, err := s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
		Ref:     "e1",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Wait for dispatch to complete.
	deadline := time.After(5 * time.Second)
	for {
		snap := s.GetMetrics()
		if snap.TasksCompleted >= 1 || snap.TasksFailed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for task dispatch")
		case <-time.After(50 * time.Millisecond):
		}
	}

	snap := s.GetMetrics()
	if snap.DispatchCount == 0 {
		t.Error("dispatch count should be > 0 after task execution")
	}
	if snap.AvgDispatchLatency <= 0 {
		t.Error("avg dispatch latency should be > 0")
	}
}
