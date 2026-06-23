package orchestrator

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/ids"
)

type mockRunner struct {
	runCalled bool
	portAvail bool
	portInfo  PortInspection
	args      []string
	env       []string
	runErr    error
}

type mockCmd struct {
	pid     int
	isAlive bool
}

func (m *mockCmd) Wait() error { return nil }
func (m *mockCmd) PID() int    { return m.pid }
func (m *mockCmd) Cancel()     {}

func (m *mockRunner) Run(ctx context.Context, binary string, args []string, env []string, stdout, stderr io.Writer) (Cmd, error) {
	m.runCalled = true
	m.args = append([]string(nil), args...)
	m.env = append([]string(nil), env...)
	if m.runErr != nil {
		return nil, m.runErr
	}
	return &mockCmd{pid: 1234, isAlive: true}, nil
}

func (m *mockRunner) InspectPort(port string) PortInspection {
	if m.portInfo != (PortInspection{}) {
		return m.portInfo
	}
	return PortInspection{Available: m.portAvail}
}

func TestLaunch_Mocked(t *testing.T) {
	old := portAvailableFunc
	portAvailableFunc = func(int) bool { return true }
	defer func() { portAvailableFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, err := o.Launch("test-prof", "9999", true, nil)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	if !runner.runCalled {
		t.Error("expected runner.Run to be called")
	}
	if len(runner.args) != 1 || runner.args[0] != "bridge" {
		t.Fatalf("expected child process args [bridge], got %v", runner.args)
	}
	if !ids.IsValidID(inst.ID, "inst") {
		t.Errorf("expected ID format inst_XXXXXXXX, got %s", inst.ID)
	}
}

func TestLaunch_PortConflict(t *testing.T) {
	old := portAvailableFunc
	portAvailableFunc = func(int) bool { return true }
	defer func() { portAvailableFunc = old }()

	runner := &mockRunner{portAvail: false}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	_, err := o.Launch("test-prof", "9999", true, nil)
	if err == nil {
		t.Fatal("expected error for unavailable port")
	}
}

func TestLaunch_PortConflictIncludesProcessDetails(t *testing.T) {
	old := portAvailableFunc
	portAvailableFunc = func(int) bool { return true }
	defer func() { portAvailableFunc = old }()

	runner := &mockRunner{portInfo: PortInspection{
		Available: false,
		PID:       4321,
		Command:   "pinchtab bridge",
	}}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		InstancePortStart: 9900,
		InstancePortEnd:   9902,
	})

	_, err := o.Launch("test-prof", "9901", true, nil)
	if err == nil {
		t.Fatal("expected error for unavailable port")
	}
	msg := err.Error()
	if !strings.Contains(msg, "instance port 9901 is already in use by pid 4321 (pinchtab bridge)") {
		t.Fatalf("expected detailed port conflict, got %q", msg)
	}
	if !strings.Contains(msg, "kill 4321") {
		t.Fatalf("expected kill suggestion, got %q", msg)
	}
}

func TestInstanceIsActive(t *testing.T) {
	tests := []struct {
		name   string
		inst   *InstanceInternal
		active bool
	}{
		{
			name: "starting",
			inst: &InstanceInternal{
				Instance: bridge.Instance{Status: "starting"},
			},
			active: true,
		},
		{
			name: "running",
			inst: &InstanceInternal{
				Instance: bridge.Instance{Status: "running"},
			},
			active: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instanceIsActive(tt.inst); got != tt.active {
				t.Errorf("instanceIsActive() = %v, want %v", got, tt.active)
			}
		})
	}
}

func TestRingBufferSince_FreshWritesReturnAll(t *testing.T) {
	rb := newRingBuffer(1024)
	if _, err := rb.Write([]byte("hello world")); err != nil {
		t.Fatalf("write: %v", err)
	}

	chunk, newOffset, reset := rb.since(0)
	if chunk != "hello world" {
		t.Fatalf("chunk = %q, want %q", chunk, "hello world")
	}
	if newOffset != uint64(len("hello world")) {
		t.Fatalf("newOffset = %d, want %d", newOffset, len("hello world"))
	}
	if reset {
		t.Fatalf("reset = true, want false for in-window read")
	}
}

func TestRingBufferSince_NothingNew(t *testing.T) {
	rb := newRingBuffer(1024)
	if _, err := rb.Write([]byte("abc")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, end, _ := rb.since(0)

	chunk, newOffset, reset := rb.since(end)
	if chunk != "" {
		t.Fatalf("chunk = %q, want empty", chunk)
	}
	if newOffset != end {
		t.Fatalf("newOffset = %d, want %d", newOffset, end)
	}
	if reset {
		t.Fatalf("reset = true, want false")
	}
}

func TestRingBufferSince_DeltaAfterAppend(t *testing.T) {
	rb := newRingBuffer(1024)
	_, _ = rb.Write([]byte("first\n"))
	_, end, _ := rb.since(0)
	_, _ = rb.Write([]byte("second\n"))

	chunk, newOffset, reset := rb.since(end)
	if chunk != "second\n" {
		t.Fatalf("chunk = %q, want %q", chunk, "second\n")
	}
	if reset {
		t.Fatalf("reset = true, want false")
	}
	if newOffset != uint64(len("first\nsecond\n")) {
		t.Fatalf("newOffset = %d, want %d", newOffset, len("first\nsecond\n"))
	}
}

func TestRingBufferSince_EvictionResyncsWithFullBuffer(t *testing.T) {
	rb := newRingBuffer(8)
	_, _ = rb.Write([]byte("abcd"))
	_, oldOffset, _ := rb.since(0) // oldOffset = 4

	// Write more than the 8-byte window so oldOffset falls strictly behind the
	// retained window start (totalWritten 14, window keeps last 8 = "ghijklmn",
	// start = 6 > oldOffset 4) → caller must resync, not append.
	_, _ = rb.Write([]byte("efghijklmn"))

	chunk, newOffset, reset := rb.since(oldOffset)
	if !reset {
		t.Fatalf("reset = false, want true after eviction past offset")
	}
	if chunk != "ghijklmn" {
		t.Fatalf("chunk = %q, want %q (full current window)", chunk, "ghijklmn")
	}
	if newOffset != 14 {
		t.Fatalf("newOffset = %d, want 14", newOffset)
	}
}

func TestLogsSince_UnknownInstanceErrors(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	if _, _, _, err := o.LogsSince("nope", 0); err == nil {
		t.Fatalf("LogsSince(unknown) err = nil, want error")
	}
}

func TestLogsSince_DeltaAfterAppends(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	rb := newRingBuffer(1024)
	o.mu.Lock()
	o.instances["inst-1"] = &InstanceInternal{logBuf: rb}
	o.mu.Unlock()

	_, _ = rb.Write([]byte("line1\n"))
	chunk, offset, reset, err := o.LogsSince("inst-1", 0)
	if err != nil {
		t.Fatalf("LogsSince: %v", err)
	}
	if chunk != "line1\n" || reset {
		t.Fatalf("initial chunk = %q reset = %v, want %q false", chunk, reset, "line1\n")
	}

	_, _ = rb.Write([]byte("line2\n"))
	chunk, _, reset, err = o.LogsSince("inst-1", offset)
	if err != nil {
		t.Fatalf("LogsSince delta: %v", err)
	}
	if chunk != "line2\n" || reset {
		t.Fatalf("delta chunk = %q reset = %v, want %q false", chunk, reset, "line2\n")
	}
}
