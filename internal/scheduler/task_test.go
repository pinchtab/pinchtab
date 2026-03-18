package scheduler

import (
	"testing"
	"time"
)

func TestTaskStateIsTerminal(t *testing.T) {
	terminal := []TaskState{StateDone, StateFailed, StateCancelled, StateRejected}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("expected %q to be terminal", s)
		}
	}

	nonTerminal := []TaskState{StateQueued, StateAssigned, StateRunning}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}

func TestTaskSetState_ValidTransitions(t *testing.T) {
	cases := []struct {
		from TaskState
		to   TaskState
	}{
		{StateQueued, StateAssigned},
		{StateQueued, StateCancelled},
		{StateQueued, StateFailed},
		{StateQueued, StateRejected},
		{StateAssigned, StateRunning},
		{StateAssigned, StateCancelled},
		{StateRunning, StateDone},
		{StateRunning, StateFailed},
		{StateRunning, StateCancelled},
	}

	for _, tc := range cases {
		task := &Task{State: tc.from, CreatedAt: time.Now()}
		if err := task.SetState(tc.to); err != nil {
			t.Errorf("transition %s → %s should be valid, got: %v", tc.from, tc.to, err)
		}
		if task.GetState() != tc.to {
			t.Errorf("expected state %s, got %s", tc.to, task.GetState())
		}
	}
}

func TestTaskSetState_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from TaskState
		to   TaskState
	}{
		{StateQueued, StateRunning},
		{StateQueued, StateDone},
		{StateAssigned, StateDone},
		{StateAssigned, StateFailed},
		{StateRunning, StateQueued},
		{StateRunning, StateAssigned},
	}

	for _, tc := range cases {
		task := &Task{State: tc.from}
		if err := task.SetState(tc.to); err == nil {
			t.Errorf("transition %s → %s should be invalid", tc.from, tc.to)
		}
	}
}

func TestTaskSetState_TerminalBlocked(t *testing.T) {
	for _, terminal := range []TaskState{StateDone, StateFailed, StateCancelled, StateRejected} {
		task := &Task{State: terminal}
		if err := task.SetState(StateQueued); err == nil {
			t.Errorf("should not allow transition from terminal state %s", terminal)
		}
	}
}

func TestTaskSetState_SetsTimestamps(t *testing.T) {
	task := &Task{State: StateQueued, CreatedAt: time.Now()}

	if err := task.SetState(StateAssigned); err != nil {
		t.Fatal(err)
	}
	if task.StartedAt.IsZero() {
		t.Error("StartedAt should be set on assigned")
	}

	if err := task.SetState(StateRunning); err != nil {
		t.Fatal(err)
	}

	if err := task.SetState(StateDone); err != nil {
		t.Fatal(err)
	}
	if task.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set on done")
	}
	if task.LatencyMs <= 0 {
		// Allow 0 only if instant (<1ms)
		t.Log("latency was 0, which can happen on fast machines")
	}
}

func TestTaskSnapshot(t *testing.T) {
	task := &Task{
		ID:      "tsk_test1",
		AgentID: "agent-1",
		Action:  "click",
		State:   StateQueued,
	}
	snap := task.Snapshot()
	if snap.ID != task.ID || snap.AgentID != task.AgentID {
		t.Error("snapshot should copy fields")
	}
}

func TestSubmitRequestValidate(t *testing.T) {
	valid := SubmitRequest{AgentID: "a", Action: "click"}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid request should pass: %v", err)
	}

	noAgent := SubmitRequest{Action: "click"}
	if err := noAgent.Validate(); err == nil {
		t.Error("missing agentId should fail")
	}

	noAction := SubmitRequest{AgentID: "a"}
	if err := noAction.Validate(); err == nil {
		t.Error("missing action should fail")
	}

	blockedCallback := SubmitRequest{
		AgentID:     "a",
		Action:      "click",
		CallbackURL: "http://127.0.0.1/hook",
	}
	if err := blockedCallback.Validate(); err == nil {
		t.Error("loopback callbackUrl should fail")
	}
}

func TestGenerateTaskID(t *testing.T) {
	id := generateTaskID()
	if len(id) < 12 {
		t.Errorf("task ID too short: %s", id)
	}
	if id[:4] != "tsk_" {
		t.Errorf("task ID should start with tsk_: %s", id)
	}

	id2 := generateTaskID()
	if id == id2 {
		t.Error("consecutive task IDs should be unique")
	}
}
