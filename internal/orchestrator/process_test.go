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
