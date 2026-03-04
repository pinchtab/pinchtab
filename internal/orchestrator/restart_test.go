package orchestrator

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// ---------------------------------------------------------------------------
// Test helpers – controllable Cmd and Runner for restart scenarios
// ---------------------------------------------------------------------------

// blockingCmd is a Cmd whose Wait() blocks until the caller sends on waitCh.
type blockingCmd struct {
	pid    int
	waitCh chan error
}

func (c *blockingCmd) Wait() error { return <-c.waitCh }
func (c *blockingCmd) PID() int    { return c.pid }
func (c *blockingCmd) Cancel()     {}

// restartRunner keeps track of Run() calls, exposes blockingCmds for crash
// simulation, and tracks which PIDs are "alive" for processAliveFunc.
type restartRunner struct {
	mu       sync.Mutex
	cmds     []*blockingCmd
	livePIDs map[int]bool
}

func newRestartRunner() *restartRunner {
	return &restartRunner{livePIDs: make(map[int]bool)}
}

func (r *restartRunner) Run(_ context.Context, _ string, _ []string, _, _ io.Writer) (Cmd, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd := &blockingCmd{pid: 2000 + len(r.cmds), waitCh: make(chan error, 1)}
	r.cmds = append(r.cmds, cmd)
	r.livePIDs[cmd.pid] = true
	return cmd, nil
}

func (r *restartRunner) IsPortAvailable(_ string) bool { return true }

func (r *restartRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cmds)
}

// crashN signals the Nth process to exit and marks its PID dead.
func (r *restartRunner) crashN(idx int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < len(r.cmds) {
		delete(r.livePIDs, r.cmds[idx].pid)
		r.cmds[idx].waitCh <- nil
	}
}

func (r *restartRunner) isAlive(pid int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.livePIDs[pid]
}

// ---------------------------------------------------------------------------
// Polling helpers
// ---------------------------------------------------------------------------

// waitForStatus polls until the instance has the desired status (via List()).
func waitForStatus(t *testing.T, o *Orchestrator, id, want string, deadline time.Duration) bridge.Instance {
	t.Helper()
	dl := time.After(deadline)
	for {
		select {
		case <-dl:
			list := o.List()
			for _, inst := range list {
				if inst.ID == id {
					t.Fatalf("timed out waiting for status %q on %s (last: %q)", want, id, inst.Status)
				}
			}
			t.Fatalf("timed out waiting for status %q on %s (instance not found)", want, id)
		default:
		}
		for _, inst := range o.List() {
			if inst.ID == id && inst.Status == want {
				return inst
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForRunCalls polls until the runner has been called at least n times.
func waitForRunCalls(t *testing.T, runner *restartRunner, n int, deadline time.Duration) {
	t.Helper()
	dl := time.After(deadline)
	for {
		select {
		case <-dl:
			t.Fatalf("timed out waiting for %d Run calls (got %d)", n, runner.callCount())
		default:
		}
		if runner.callCount() >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForEvent polls the event list until the desired event type appears.
func waitForEvent(t *testing.T, mu *sync.Mutex, events *[]string, want string, deadline time.Duration) {
	t.Helper()
	dl := time.After(deadline)
	for {
		select {
		case <-dl:
			mu.Lock()
			got := make([]string, len(*events))
			copy(got, *events)
			mu.Unlock()
			t.Fatalf("timed out waiting for event %q (got: %v)", want, got)
		default:
		}
		mu.Lock()
		for _, e := range *events {
			if e == want {
				mu.Unlock()
				return
			}
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
}

// healthServer starts a test HTTP server that responds 200 to any path.
// Returns the server (caller must Close) and the port string.
func healthServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Extract port from "http://127.0.0.1:PORT"
	parts := strings.Split(srv.URL, ":")
	port := parts[len(parts)-1]
	return srv, port
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestConfigureRestart(t *testing.T) {
	runner := newRestartRunner()
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	// Defaults
	if !o.autoRestart {
		t.Error("default autoRestart should be true")
	}
	if o.maxRestarts != 3 {
		t.Errorf("default maxRestarts = %d, want 3", o.maxRestarts)
	}
	if o.restartBackoff != 2*time.Second {
		t.Errorf("default restartBackoff = %v, want 2s", o.restartBackoff)
	}
	if o.stableAfter != 5*time.Minute {
		t.Errorf("default stableAfter = %v, want 5m", o.stableAfter)
	}

	// Override
	o.ConfigureRestart(false, 10, 500*time.Millisecond, 1*time.Minute)
	if o.autoRestart {
		t.Error("autoRestart should be false after ConfigureRestart(false, ...)")
	}
	if o.maxRestarts != 10 {
		t.Errorf("maxRestarts = %d, want 10", o.maxRestarts)
	}
	if o.restartBackoff != 500*time.Millisecond {
		t.Errorf("restartBackoff = %v, want 500ms", o.restartBackoff)
	}
	if o.stableAfter != 1*time.Minute {
		t.Errorf("stableAfter = %v, want 1m", o.stableAfter)
	}
}

func TestAutoRestart_CrashedInstanceRestarted(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ConfigureRestart(true, 3, 1*time.Millisecond, 5*time.Minute)

	inst, err := o.Launch("restart-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	if runner.callCount() != 1 {
		t.Fatalf("expected 1 Run call before crash, got %d", runner.callCount())
	}

	// Crash the first process
	runner.crashN(0)

	// Detect restart via runner call count, then wait for running
	waitForRunCalls(t, runner, 2, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	if runner.callCount() != 2 {
		t.Errorf("expected 2 Run calls after one restart, got %d", runner.callCount())
	}
}

func TestAutoRestart_MaxRestartsExceeded(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ConfigureRestart(true, 2, 1*time.Millisecond, 1*time.Hour)

	inst, err := o.Launch("max-restart-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash #1 → auto-restart
	runner.crashN(0)
	waitForRunCalls(t, runner, 2, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash #2 → auto-restart (still within limit)
	runner.crashN(1)
	waitForRunCalls(t, runner, 3, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash #3 → should stop (exceeded max 2 restarts)
	runner.crashN(2)
	waitForStatus(t, o, inst.ID, "stopped", 20*time.Second)

	// 1 initial + 2 restarts = 3 total
	if calls := runner.callCount(); calls != 3 {
		t.Errorf("expected 3 total Run calls (1 + 2 restarts), got %d", calls)
	}
}

func TestAutoRestart_DisabledSkipsRestart(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ConfigureRestart(false, 3, 1*time.Millisecond, 5*time.Minute)

	inst, err := o.Launch("no-restart-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash → should NOT restart because autoRestart is false
	runner.crashN(0)
	waitForStatus(t, o, inst.ID, "stopped", 20*time.Second)

	if runner.callCount() != 1 {
		t.Errorf("expected 1 Run call (no restart), got %d", runner.callCount())
	}
}

func TestAutoRestart_DeliberateStopSkipsRestart(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ConfigureRestart(true, 3, 1*time.Millisecond, 5*time.Minute)

	inst, err := o.Launch("stop-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Simulate deliberate stop: set status to "stopping" directly.
	// This is what Orchestrator.Stop() does before killing the process.
	o.mu.Lock()
	internal := o.instances[inst.ID]
	internal.Status = "stopping"
	o.mu.Unlock()

	// Signal process exit
	runner.crashN(0)

	// Wait for monitor to process
	time.Sleep(300 * time.Millisecond)

	// Should NOT restart because status was "stopping" (deliberate stop)
	if runner.callCount() != 1 {
		t.Errorf("expected 1 Run call (no restart after deliberate stop), got %d", runner.callCount())
	}
}

func TestAutoRestart_EventsEmitted(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ConfigureRestart(true, 2, 1*time.Millisecond, 1*time.Hour)

	var mu sync.Mutex
	var events []string
	o.OnEvent(func(evt InstanceEvent) {
		mu.Lock()
		events = append(events, evt.Type)
		mu.Unlock()
	})

	inst, err := o.Launch("event-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash #1
	runner.crashN(0)

	// Wait for events using event polling (avoids transient status race)
	waitForEvent(t, &mu, &events, "instance.restarting", 10*time.Second)
	waitForEvent(t, &mu, &events, "instance.restarted", 10*time.Second)

	// Wait for instance to become running after restart
	waitForRunCalls(t, runner, 2, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)
}

func TestAutoRestart_StableAfterResetsCounter(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	// max 1 restart, stableAfter very short so counter resets quickly
	o.ConfigureRestart(true, 1, 1*time.Millisecond, 50*time.Millisecond)

	inst, err := o.Launch("stable-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash #1 → restart (count 0→1)
	runner.crashN(0)
	waitForRunCalls(t, runner, 2, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Wait longer than stableAfter so the counter resets
	time.Sleep(100 * time.Millisecond)

	// Crash #2 → should restart again because counter was reset
	runner.crashN(1)
	waitForRunCalls(t, runner, 3, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	if runner.callCount() != 3 {
		t.Errorf("expected 3 Run calls (1 initial + 2 restarts with stable reset), got %d", runner.callCount())
	}
}

func TestAutoRestart_InstanceInternalFieldsUpdated(t *testing.T) {
	runner := newRestartRunner()
	old := processAliveFunc
	processAliveFunc = runner.isAlive
	defer func() { processAliveFunc = old }()

	srv, port := healthServer(t)
	defer srv.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ConfigureRestart(true, 3, 1*time.Millisecond, 5*time.Minute)

	inst, err := o.Launch("field-test", port, true)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Crash
	runner.crashN(0)
	waitForRunCalls(t, runner, 2, 10*time.Second)
	waitForStatus(t, o, inst.ID, "running", 20*time.Second)

	// Check internal state
	o.mu.RLock()
	internal, ok := o.instances[inst.ID]
	if !ok {
		o.mu.RUnlock()
		t.Fatal("instance not found in map after restart")
	}
	rc := internal.restartCount
	lc := internal.lastCrash
	o.mu.RUnlock()

	if rc != 1 {
		t.Errorf("restartCount = %d, want 1", rc)
	}
	if lc.IsZero() {
		t.Error("lastCrash should be set after a crash")
	}
}
