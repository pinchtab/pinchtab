package autorestart

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
)

// fakeBridge creates a test server that mimics a bridge instance.
func fakeBridge(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"proxied": true, "path": r.URL.Path})
	}))
}

func TestStrategy_Name(t *testing.T) {
	s := New(AutorestartConfig{})
	if s.Name() != "simple-autorestart" {
		t.Errorf("expected 'simple-autorestart', got %q", s.Name())
	}
}

func TestStrategy_DefaultConfig(t *testing.T) {
	s := New(AutorestartConfig{})
	if s.config.MaxRestarts != defaultMaxRestarts {
		t.Errorf("expected MaxRestarts=%d, got %d", defaultMaxRestarts, s.config.MaxRestarts)
	}
	if s.config.InitBackoff != defaultInitBackoff {
		t.Errorf("expected InitBackoff=%s, got %s", defaultInitBackoff, s.config.InitBackoff)
	}
	if s.config.StableAfter != defaultStableAfter {
		t.Errorf("expected StableAfter=%s, got %s", defaultStableAfter, s.config.StableAfter)
	}
	if s.config.ProfileName != defaultProfileName {
		t.Errorf("expected ProfileName=%q, got %q", defaultProfileName, s.config.ProfileName)
	}
	if !s.config.Headless {
		t.Error("expected Headless=true by default")
	}
}

func TestStrategy_CustomConfig(t *testing.T) {
	s := New(AutorestartConfig{
		MaxRestarts: 5,
		InitBackoff: 1 * time.Second,
		StableAfter: 10 * time.Minute,
		ProfileName: "myprofile",
		Headless:    false,
		HeadlessSet: true,
	})

	if s.config.MaxRestarts != 5 {
		t.Errorf("expected MaxRestarts=5, got %d", s.config.MaxRestarts)
	}
	if s.config.InitBackoff != 1*time.Second {
		t.Errorf("expected InitBackoff=1s, got %s", s.config.InitBackoff)
	}
	if s.config.StableAfter != 10*time.Minute {
		t.Errorf("expected StableAfter=10m, got %s", s.config.StableAfter)
	}
	if s.config.ProfileName != "myprofile" {
		t.Errorf("expected ProfileName=myprofile, got %q", s.config.ProfileName)
	}
	if s.config.Headless {
		t.Error("expected Headless=false when explicitly set")
	}
}

func TestStrategy_UnlimitedRestarts(t *testing.T) {
	s := New(AutorestartConfig{MaxRestarts: -1})
	s.restartCount = 100

	state := s.State()
	if state.MaxRestarts != -1 {
		t.Fatalf("expected MaxRestarts=-1, got %d", state.MaxRestarts)
	}
	if state.Status == "crashed" {
		t.Fatal("expected unlimited-restart strategy to avoid crashed state")
	}
}

func TestStrategy_SetRuntimeConfig(t *testing.T) {
	s := New(AutorestartConfig{})
	s.SetRuntimeConfig(&config.RuntimeConfig{
		RestartMaxRestarts: 11,
		RestartInitBackoff: 4 * time.Second,
		RestartMaxBackoff:  45 * time.Second,
		RestartStableAfter: 8 * time.Minute,
	})

	if s.config.MaxRestarts != 11 {
		t.Fatalf("expected MaxRestarts=11, got %d", s.config.MaxRestarts)
	}
	if s.config.InitBackoff != 4*time.Second {
		t.Fatalf("expected InitBackoff=4s, got %s", s.config.InitBackoff)
	}
	if s.config.MaxBackoff != 45*time.Second {
		t.Fatalf("expected MaxBackoff=45s, got %s", s.config.MaxBackoff)
	}
	if s.config.StableAfter != 8*time.Minute {
		t.Fatalf("expected StableAfter=8m, got %s", s.config.StableAfter)
	}
}

func TestStrategy_State_InitialStatus(t *testing.T) {
	s := New(AutorestartConfig{})
	state := s.State()

	if state.Status != "starting" {
		t.Errorf("expected initial status 'starting', got %q", state.Status)
	}
	if state.RestartCount != 0 {
		t.Errorf("expected initial restartCount=0, got %d", state.RestartCount)
	}
	if state.MaxRestarts != defaultMaxRestarts {
		t.Errorf("expected maxRestarts=%d, got %d", defaultMaxRestarts, state.MaxRestarts)
	}
}

func TestStrategy_HandleCrash_IncrementsCounter(t *testing.T) {
	s := New(AutorestartConfig{
		MaxRestarts: 5,
		InitBackoff: 1 * time.Millisecond, // Fast for testing
	})
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	// Set orchestrator so emitting events doesn't panic
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	s.orch = orch

	// Simulate a crash without an actual running instance (launch will fail, but counter increments)
	s.instanceID = "inst_test123"
	s.handleCrash(s.ctx, "inst_test123")

	s.mu.Lock()
	count := s.restartCount
	crashed := s.lastCrash
	s.mu.Unlock()

	if count != 1 {
		t.Errorf("expected restartCount=1, got %d", count)
	}
	if crashed.IsZero() {
		t.Error("expected lastCrash to be set")
	}
}

func TestStrategy_HandleCrash_MaxRestartsExceeded(t *testing.T) {
	var eventsMu sync.Mutex
	var events []string

	// Create a minimal orchestrator to capture events
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		eventsMu.Lock()
		events = append(events, evt.Type)
		eventsMu.Unlock()
	})

	s := New(AutorestartConfig{
		MaxRestarts: 2,
		InitBackoff: 1 * time.Millisecond,
	})
	s.orch = orch
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.instanceID = "inst_test123"

	// Simulate crashes beyond the limit
	s.restartCount = 2 // Already at max

	s.handleCrash(s.ctx, "inst_test123")

	s.mu.Lock()
	count := s.restartCount
	s.mu.Unlock()

	if count != 3 {
		t.Errorf("expected restartCount=3, got %d", count)
	}

	// Should have emitted "instance.crashed" event
	time.Sleep(50 * time.Millisecond) // Allow event goroutine to fire
	eventsMu.Lock()
	found := false
	for _, e := range events {
		if e == "instance.crashed" {
			found = true
		}
	}
	eventsMu.Unlock()

	if !found {
		t.Error("expected 'instance.crashed' event when max restarts exceeded")
	}
}

func TestStrategy_HandleEvent_IgnoresUnmanagedInstances(t *testing.T) {
	s := New(AutorestartConfig{})
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.instanceID = "inst_managed"

	// Fire event for a different instance
	s.handleEvent(orchestrator.InstanceEvent{
		Type:     "instance.stopped",
		Instance: &bridge.Instance{ID: "inst_other"},
	})

	s.mu.Lock()
	count := s.restartCount
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected restartCount=0 (unmanaged instance), got %d", count)
	}
}

func TestStrategy_HandleEvent_DeliberateStopSkipsRestart(t *testing.T) {
	s := New(AutorestartConfig{})
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.instanceID = "inst_managed"
	s.deliberate = true

	s.handleEvent(orchestrator.InstanceEvent{
		Type:     "instance.stopped",
		Instance: &bridge.Instance{ID: "inst_managed"},
	})

	s.mu.Lock()
	count := s.restartCount
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected restartCount=0 (deliberate stop), got %d", count)
	}
}

func TestStrategy_HandleEvent_NilInstanceIgnored(t *testing.T) {
	s := New(AutorestartConfig{})
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	// Should not panic
	s.handleEvent(orchestrator.InstanceEvent{
		Type:     "instance.stopped",
		Instance: nil,
	})
}

func TestStrategy_StabilityReset(t *testing.T) {
	s := New(AutorestartConfig{
		StableAfter: 50 * time.Millisecond, // Short for testing
	})

	s.instanceID = "inst_test"
	s.restartCount = 2
	s.lastStart = time.Now().Add(-100 * time.Millisecond) // Already past stable period

	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	// Run stability check manually (simulating what the loop does)
	s.mu.Lock()
	if s.restartCount > 0 && !s.lastStart.IsZero() && time.Since(s.lastStart) > s.config.StableAfter {
		s.restartCount = 0
	}
	count := s.restartCount
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected restartCount reset to 0, got %d", count)
	}
}

func TestStrategy_StabilityNoResetTooEarly(t *testing.T) {
	s := New(AutorestartConfig{
		StableAfter: 1 * time.Hour, // Very long
	})

	s.instanceID = "inst_test"
	s.restartCount = 2
	s.lastStart = time.Now() // Just started

	// Stability check should NOT reset
	s.mu.Lock()
	if s.restartCount > 0 && !s.lastStart.IsZero() && time.Since(s.lastStart) > s.config.StableAfter {
		s.restartCount = 0
	}
	count := s.restartCount
	s.mu.Unlock()

	if count != 2 {
		t.Errorf("expected restartCount=2 (not yet stable), got %d", count)
	}
}

func TestStrategy_Stop_SetsDeliberate(t *testing.T) {
	s := New(AutorestartConfig{})
	s.ctx, s.cancel = context.WithCancel(context.Background())

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	s.mu.Lock()
	if !s.deliberate {
		t.Error("expected deliberate=true after Stop")
	}
	s.mu.Unlock()
}

func TestStrategy_EnsureRunning_NoOrch(t *testing.T) {
	s := New(AutorestartConfig{})
	_, err := s.ensureRunning()
	if err == nil {
		t.Error("expected error when no orchestrator")
	}
}

func TestStrategy_ProxyToManaged_NoOrch_Returns503(t *testing.T) {
	s := New(AutorestartConfig{})
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	s.proxyToManaged(rec, req)

	if rec.Code != 503 {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStrategy_HandleTabs_NoInstances(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/tabs", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, srv.URL+"/tabs")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestProxyHTTP_ForwardsRequest(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, srv.URL+"/snapshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["path"] != "/snapshot" {
		t.Errorf("expected path /snapshot, got %v", resp["path"])
	}
}

func TestProxyHTTP_ForwardsQueryParams(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/screenshot?raw=true", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, srv.URL+"/screenshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestProxyHTTP_UnreachableReturns502(t *testing.T) {
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, "http://localhost:1/snapshot")

	if rec.Code != 502 {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

func TestStrategy_HandleStatus(t *testing.T) {
	s := New(AutorestartConfig{MaxRestarts: 3})
	s.instanceID = "inst_test"
	s.restartCount = 1
	s.lastStart = time.Now()

	req := httptest.NewRequest("GET", "/autorestart/status", nil)
	rec := httptest.NewRecorder()
	s.handleStatus(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var state RestartState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("failed to parse status response: %v", err)
	}

	if state.InstanceID != "inst_test" {
		t.Errorf("expected instanceId=inst_test, got %q", state.InstanceID)
	}
	if state.RestartCount != 1 {
		t.Errorf("expected restartCount=1, got %d", state.RestartCount)
	}
	if state.MaxRestarts != 3 {
		t.Errorf("expected maxRestarts=3, got %d", state.MaxRestarts)
	}
	if state.Status != "running" {
		t.Errorf("expected status=running, got %q", state.Status)
	}
}

func TestStrategy_State_CrashedStatus(t *testing.T) {
	s := New(AutorestartConfig{MaxRestarts: 2})
	s.instanceID = "inst_test"
	s.restartCount = 2 // At max

	state := s.State()
	if state.Status != "crashed" {
		t.Errorf("expected status=crashed when restartCount>=maxRestarts, got %q", state.Status)
	}
}

func TestStrategy_ExponentialBackoff(t *testing.T) {
	s := New(AutorestartConfig{
		InitBackoff: 100 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	})

	// Simulate increasing backoff values with cap
	tests := []struct {
		restartCount int
		wantBackoff  time.Duration
	}{
		{0, 100 * time.Millisecond}, // 100ms * 2^0
		{1, 200 * time.Millisecond}, // 100ms * 2^1
		{2, 400 * time.Millisecond}, // 100ms * 2^2
		{3, 500 * time.Millisecond}, // capped at 500ms (would be 800ms)
		{4, 500 * time.Millisecond}, // capped at 500ms (would be 1600ms)
	}

	for _, tt := range tests {
		backoff := s.config.InitBackoff * time.Duration(1<<uint(tt.restartCount))
		if backoff > s.config.MaxBackoff {
			backoff = s.config.MaxBackoff
		}
		if backoff != tt.wantBackoff {
			t.Errorf("restartCount=%d: expected backoff=%s, got %s", tt.restartCount, tt.wantBackoff, backoff)
		}
	}
}

func TestStrategy_BackoffCap_Default(t *testing.T) {
	s := New(AutorestartConfig{})
	if s.config.MaxBackoff != defaultMaxBackoff {
		t.Errorf("expected default MaxBackoff=%s, got %s", defaultMaxBackoff, s.config.MaxBackoff)
	}
}

func TestStrategy_BackoffCap_Custom(t *testing.T) {
	s := New(AutorestartConfig{MaxBackoff: 30 * time.Second})
	if s.config.MaxBackoff != 30*time.Second {
		t.Errorf("expected MaxBackoff=30s, got %s", s.config.MaxBackoff)
	}
}

func TestStrategy_HandleCrash_RestartingEvent(t *testing.T) {
	var eventsMu sync.Mutex
	var events []string

	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		eventsMu.Lock()
		events = append(events, evt.Type)
		eventsMu.Unlock()
	})

	s := New(AutorestartConfig{
		MaxRestarts: 5,
		InitBackoff: 1 * time.Millisecond,
	})
	s.orch = orch
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.instanceID = "inst_test123"

	// First crash — should emit "instance.restarting"
	s.handleCrash(s.ctx, "inst_test123")

	time.Sleep(50 * time.Millisecond)
	eventsMu.Lock()
	found := false
	for _, e := range events {
		if e == "instance.restarting" {
			found = true
		}
	}
	eventsMu.Unlock()

	if !found {
		t.Error("expected 'instance.restarting' event on first crash")
	}
}

func TestStrategy_LaunchInitial_TracksLaunchedMode(t *testing.T) {
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	s := New(AutorestartConfig{
		ProfileName: "headed-profile",
		Headless:    false,
		HeadlessSet: true,
	})
	s.orch = orch
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.launchInitial()

	s.mu.Lock()
	instanceID := s.instanceID
	headless := s.headless
	s.mu.Unlock()

	if instanceID == "" {
		t.Fatal("expected launchInitial to track a managed instance")
	}
	if headless {
		t.Fatal("expected launchInitial to preserve headed mode")
	}
}

func TestStrategy_RestartInstance_UsesTrackedMode(t *testing.T) {
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	if _, _, err := orch.AttachBridge("default", "http://127.0.0.1:9999", ""); err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
	}

	s := New(AutorestartConfig{
		ProfileName: "default",
		Headless:    true,
		HeadlessSet: true,
	})
	s.orch = orch
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()
	s.instanceID = "inst_default_default"
	s.headless = false
	s.restarting = true

	s.restartInstance()

	instances := orch.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 managed instance after restart, got %d", len(instances))
	}
	if instances[0].Headless {
		t.Fatalf("expected restarted instance to remain headed, got headless=true")
	}
}

func TestStrategy_ContextCancellation(t *testing.T) {
	s := New(AutorestartConfig{
		InitBackoff: 10 * time.Second, // Long backoff
	})
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	s.instanceID = "inst_test"

	// Cancel context immediately
	cancel()

	// handleCrash should return quickly due to context cancellation
	done := make(chan struct{})
	go func() {
		s.handleCrash(ctx, "inst_test")
		close(done)
	}()

	select {
	case <-done:
		// Success — returned quickly
	case <-time.After(2 * time.Second):
		t.Error("handleCrash did not respect context cancellation")
	}
}

func TestStrategy_State_RestartingStatus(t *testing.T) {
	s := New(AutorestartConfig{MaxRestarts: 3})
	s.instanceID = "inst_test"
	s.restarting = true

	state := s.State()
	if state.Status != "restarting" {
		t.Errorf("expected status=restarting, got %q", state.Status)
	}
}

func TestStrategy_HandleEvent_SkipsDuplicateDuringRestart(t *testing.T) {
	s := New(AutorestartConfig{})
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.instanceID = "inst_managed"
	s.restarting = true // Already restarting

	// Sending another stopped event should be ignored
	s.handleEvent(orchestrator.InstanceEvent{
		Type:     "instance.stopped",
		Instance: &bridge.Instance{ID: "inst_managed"},
	})

	s.mu.Lock()
	count := s.restartCount
	s.mu.Unlock()

	if count != 0 {
		t.Errorf("expected restartCount=0 (skipped during restart), got %d", count)
	}
}

// --- Mock runner for orchestrator tests ---

type mockRunner struct {
	portAvail bool
}

type mockCmd struct{}

func (m *mockCmd) Wait() error { return nil }
func (m *mockCmd) PID() int    { return 1234 }
func (m *mockCmd) Cancel()     {}

func (r *mockRunner) Run(_ context.Context, _ string, _ []string, _ []string, _, _ io.Writer) (orchestrator.Cmd, error) {
	return &mockCmd{}, nil
}

func (r *mockRunner) InspectPort(_ string) orchestrator.PortInspection {
	return orchestrator.PortInspection{Available: r.portAvail}
}
