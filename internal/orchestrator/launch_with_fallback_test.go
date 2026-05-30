package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type scriptedOutcome struct {
	target      string
	succeed     bool
	failReason  LaunchFailureReason
	errOnLaunch error
}

type fakeLauncher struct {
	mu         sync.Mutex
	plan       []scriptedOutcome
	calls      []LaunchOptions
	createdIDs []string
	tornDown   []string
	idCounter  int
	orch       *Orchestrator
	outcomes   map[string]scriptedOutcome
}

func (fl *fakeLauncher) ResolveTarget(candidate string) (ResolvedLaunchTarget, error) {
	if strings.Contains(candidate, "cloak") {
		return ResolvedLaunchTarget{Name: candidate, Provider: config.BrowserCloak}, nil
	}
	switch candidate {
	case "chrome", "chrome-local", "backup":
		return ResolvedLaunchTarget{Name: candidate, Provider: config.BrowserChrome}, nil
	default:
		return ResolvedLaunchTarget{}, fmt.Errorf("unknown target %q", candidate)
	}
}

func (fl *fakeLauncher) Launch(name, port string, headless bool, opts LaunchOptions) (*bridge.Instance, error) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if len(fl.calls) >= len(fl.plan) {
		return nil, fmt.Errorf("fakeLauncher: unexpected extra call (call #%d, plan has %d entries)", len(fl.calls)+1, len(fl.plan))
	}
	step := fl.plan[len(fl.calls)]
	fl.calls = append(fl.calls, opts)

	if step.errOnLaunch != nil {
		return nil, step.errOnLaunch
	}

	fl.idCounter++
	id := fmt.Sprintf("inst_fake_%02d", fl.idCounter)
	fl.createdIDs = append(fl.createdIDs, id)
	if fl.outcomes == nil {
		fl.outcomes = make(map[string]scriptedOutcome)
	}
	fl.outcomes[id] = step

	instance := bridge.Instance{
		ID:          id,
		ProfileName: name,
		Port:        port,
		Headless:    headless,
		Mode:        bridge.ModeFromHeadless(headless),
		Status:      "starting",
		StartTime:   time.Now(),
		Browser:     opts.Browser,
	}
	if fl.orch != nil {
		internal := &InstanceInternal{
			Instance: instance,
			browser:  opts.Browser,
		}
		fl.orch.mu.Lock()
		fl.orch.instances[id] = internal
		fl.orch.mu.Unlock()
	}

	snapshot := instance
	return &snapshot, nil
}

func (fl *fakeLauncher) WaitForLaunchOutcome(instanceID string, timeout time.Duration) (LaunchOutcome, error) {
	fl.mu.Lock()
	step, ok := fl.outcomes[instanceID]
	fl.mu.Unlock()
	if !ok {
		return LaunchOutcome{}, fmt.Errorf("unknown instance %q", instanceID)
	}

	if step.succeed {
		if fl.orch != nil {
			fl.orch.mu.Lock()
			if internal, ok := fl.orch.instances[instanceID]; ok {
				internal.Status = "running"
			}
			fl.orch.mu.Unlock()
		}
		return LaunchOutcome{Status: "running"}, nil
	}

	if fl.orch != nil {
		fl.orch.mu.Lock()
		if internal, ok := fl.orch.instances[instanceID]; ok {
			internal.Status = "error"
			internal.lastFailureReason = step.failReason
			internal.Error = fmt.Sprintf("scripted failure: %s", string(step.failReason))
		}
		fl.orch.mu.Unlock()
	}
	return LaunchOutcome{Status: "error", Reason: step.failReason}, nil
}

func (fl *fakeLauncher) TearDownFailedAttempt(instanceID string) {
	fl.mu.Lock()
	fl.tornDown = append(fl.tornDown, instanceID)
	fl.mu.Unlock()
	if fl.orch != nil {
		fl.orch.mu.Lock()
		delete(fl.orch.instances, instanceID)
		fl.orch.mu.Unlock()
	}
}

func fakeLauncherFor(plan []scriptedOutcome) *fakeLauncher {
	return &fakeLauncher{plan: plan, outcomes: make(map[string]scriptedOutcome)}
}

func fakeLauncherForOrch(t *testing.T, o *Orchestrator, plan []scriptedOutcome) *fakeLauncher {
	t.Helper()
	fl := fakeLauncherFor(plan)
	fl.orch = o
	o.fallbackLauncher = fl
	return fl
}

func launchWithFallbackForTest(fl *fakeLauncher, candidates []string) (*PlannedLaunch, error) {
	return LaunchPlanner{
		Launcher: fl,
		Timeout:  time.Second,
	}.LaunchWithFallback("prof", "9000", true, candidates, LaunchOptions{})
}

func fastPolling(t *testing.T) {
	t.Helper()
	prev := launchOutcomePollInterval
	launchOutcomePollInterval = time.Millisecond
	t.Cleanup(func() { launchOutcomePollInterval = prev })
}

func newFallbackTestOrch(t *testing.T) *Orchestrator {
	t.Helper()
	return &Orchestrator{
		instances:     make(map[string]*InstanceInternal),
		portAllocator: NewPortAllocator(9000, 9100),
		runtimeCfg: &config.RuntimeConfig{
			DefaultBrowser: config.BrowserChrome,
			Targets: config.BrowserTargetsConfig{
				"chrome": config.BrowserTargetConfig{Provider: config.BrowserChrome},
				"cloak":  config.BrowserTargetConfig{Provider: config.BrowserCloak},
				"backup": config.BrowserTargetConfig{Provider: config.BrowserChrome},
			},
			DefaultTarget: "chrome",
		},
	}
}

func TestLaunchWithFallback_FirstCandidateSucceeds(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{
		{target: "chrome", succeed: true},
	})

	planned, err := launchWithFallbackForTest(fl, []string{"chrome"})
	if err != nil {
		t.Fatalf("LaunchWithFallback returned error: %v", err)
	}
	inst := planned.Instance
	if inst == nil {
		t.Fatal("expected non-nil instance")
	}
	if inst.FallbackFrom != "" || inst.FallbackReason != "" {
		t.Errorf("expected no fallback metadata, got from=%q reason=%q", inst.FallbackFrom, inst.FallbackReason)
	}
	if inst.Browser != config.BrowserChrome {
		t.Errorf("Browser = %q, want chrome", inst.Browser)
	}
	if len(fl.calls) != 1 {
		t.Errorf("expected 1 launch call, got %d", len(fl.calls))
	}
}

func TestLaunchWithFallback_FirstFailsSecondSucceeds(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{
		{target: "chrome", failReason: ReasonBinaryMissing},
		{target: "cloak", succeed: true},
	})

	planned, err := launchWithFallbackForTest(fl, []string{"chrome", "cloak"})
	if err != nil {
		t.Fatalf("LaunchWithFallback returned error: %v", err)
	}
	inst := planned.Instance
	if planned.FallbackFrom != "chrome" {
		t.Errorf("FallbackFrom = %q, want chrome", planned.FallbackFrom)
	}
	if planned.FallbackReason != ReasonBinaryMissing {
		t.Errorf("FallbackReason = %q, want %q", planned.FallbackReason, ReasonBinaryMissing)
	}
	if inst.Browser != config.BrowserCloak {
		t.Errorf("Browser = %q, want cloak", inst.Browser)
	}
	if len(fl.calls) != 2 {
		t.Errorf("expected 2 launch calls, got %d", len(fl.calls))
	}

}

func TestLaunchWithFallback_StoresFallbackMetadataOnCanonicalInstance(t *testing.T) {
	fastPolling(t)
	o := newFallbackTestOrch(t)
	_ = fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome", failReason: ReasonBinaryMissing},
		{target: "cloak", succeed: true},
	})

	inst, err := o.LaunchWithFallback("prof", "9000", true, []string{"chrome", "cloak"}, LaunchOptions{})
	if err != nil {
		t.Fatalf("LaunchWithFallback returned error: %v", err)
	}
	if inst.FallbackFrom != "chrome" || inst.FallbackReason != string(ReasonBinaryMissing) {
		t.Fatalf("returned fallback metadata = from %q reason %q, want chrome/%s", inst.FallbackFrom, inst.FallbackReason, ReasonBinaryMissing)
	}

	o.mu.RLock()
	stored, ok := o.instances[inst.ID]
	o.mu.RUnlock()
	if !ok {
		t.Fatalf("running instance %q missing from orchestrator map", inst.ID)
	}
	if stored.FallbackFrom != inst.FallbackFrom || stored.FallbackReason != inst.FallbackReason {
		t.Errorf("stored fallback metadata diverged from returned snapshot: stored=%+v returned=%+v", stored.Instance, inst)
	}
}

func TestLaunchUsesConfiguredFallbackOrder(t *testing.T) {
	fastPolling(t)
	o := newFallbackTestOrch(t)
	o.runtimeCfg.FallbackOrder = []string{"cloak"}
	fl := fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome", failReason: ReasonBinaryMissing},
		{target: "cloak", succeed: true},
	})

	inst, err := o.Launch("prof", "9000", true, nil)
	if err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}
	if inst.Browser != config.BrowserCloak {
		t.Fatalf("Browser = %q, want cloak fallback", inst.Browser)
	}
	if inst.FallbackFrom != "chrome" {
		t.Fatalf("FallbackFrom = %q, want chrome", inst.FallbackFrom)
	}
	if len(fl.calls) != 2 {
		t.Fatalf("launch calls = %d, want 2", len(fl.calls))
	}
	if fl.calls[0].Browser != config.BrowserChrome || fl.calls[1].Browser != config.BrowserCloak {
		t.Fatalf("launch browser order = [%q, %q], want [chrome, cloak]", fl.calls[0].Browser, fl.calls[1].Browser)
	}
}

func TestLaunchWithFallback_AllFail_Exhaustion(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{
		{target: "chrome", failReason: ReasonStartupTimeout},
		{target: "cloak", failReason: ReasonBinaryMissing},
		{target: "backup", failReason: ReasonHealthCheckTimeout},
	})

	_, err := launchWithFallbackForTest(fl, []string{"chrome", "cloak", "backup"})
	if err == nil {
		t.Fatal("expected error on exhaustion")
	}
	msg := err.Error()
	if !strings.Contains(msg, "all browser targets failed") {
		t.Errorf("error missing exhaustion prefix: %q", msg)
	}
	for _, want := range []string{
		"chrome:" + string(ReasonStartupTimeout),
		"cloak:" + string(ReasonBinaryMissing),
		"backup:" + string(ReasonHealthCheckTimeout),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing attempt %q: %q", want, msg)
		}
	}

	if len(fl.tornDown) != 3 {
		t.Errorf("expected all failed attempts to be torn down, got %d teardowns", len(fl.tornDown))
	}
}

func TestLaunchWithFallback_NonRecoverableAbortsImmediately(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{
		{target: "chrome", failReason: ReasonUnknown},
		// Omitting a second entry triggers "unexpected extra call" if the loop continues.
	})

	_, err := launchWithFallbackForTest(fl, []string{"chrome", "cloak"})
	if err == nil {
		t.Fatal("expected non-recoverable failure to surface as error")
	}
	if !strings.Contains(err.Error(), "non-recoverably") {
		t.Errorf("error should flag non-recoverable: %q", err.Error())
	}
	var exhausted *FallbackExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("non-recoverable failure should still return FallbackExhaustedError, got %T", err)
	}
	if len(exhausted.Attempts) != 1 || exhausted.Attempts[0].Target != "chrome" {
		t.Fatalf("attempt metadata lost: %+v", exhausted.Attempts)
	}
	if len(fl.calls) != 1 {
		t.Errorf("expected exactly 1 launch attempt, got %d", len(fl.calls))
	}
}

func TestLaunchWithFallback_TearsDownFailedAttempt(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{
		{target: "chrome", failReason: ReasonProcessExited},
		{target: "cloak", succeed: true},
	})

	_, err := launchWithFallbackForTest(fl, []string{"chrome", "cloak"})
	if err != nil {
		t.Fatalf("LaunchWithFallback returned error: %v", err)
	}

	if len(fl.createdIDs) != 2 {
		t.Fatalf("expected 2 instances created, got %d", len(fl.createdIDs))
	}
	if len(fl.tornDown) != 1 || fl.tornDown[0] != fl.createdIDs[0] {
		t.Errorf("failed attempt teardown = %v, want [%s]", fl.tornDown, fl.createdIDs[0])
	}
	if fl.tornDown[0] == fl.createdIDs[1] {
		t.Errorf("running instance %q was torn down", fl.createdIDs[1])
	}
}

func TestTearDownFailedAttempt_DetachesBeforeAsyncStop(t *testing.T) {
	oldAlive := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	t.Cleanup(func() { processAliveFunc = oldAlive })

	o := newFallbackTestOrch(t)
	failed := &InstanceInternal{
		Instance: bridge.Instance{
			ID:          "failed",
			ProfileName: "prof",
			Port:        "9001",
			Status:      "error",
		},
	}
	activeSameProfile := &InstanceInternal{
		Instance: bridge.Instance{
			ID:          "active",
			ProfileName: "prof",
			Port:        "9002",
			Status:      "running",
		},
		cmd: &mockCmd{pid: 99, isAlive: true},
	}
	o.instances[failed.ID] = failed
	o.instances[activeSameProfile.ID] = activeSameProfile

	o.tearDownFailedAttempt(failed.ID)

	o.mu.RLock()
	_, failedPresent := o.instances[failed.ID]
	_, activePresent := o.instances[activeSameProfile.ID]
	o.mu.RUnlock()
	if failedPresent {
		t.Fatal("failed attempt remained in orchestrator map")
	}
	if !activePresent {
		t.Fatal("active same-profile instance was removed")
	}
}

func TestLaunchWithFallback_UnknownCandidateIsHardError(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{})

	_, err := launchWithFallbackForTest(fl, []string{"does-not-exist", "chrome"})
	if err == nil {
		t.Fatal("expected hard error for unknown candidate")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should reference the offending name: %q", err.Error())
	}
	if len(fl.calls) != 0 {
		t.Errorf("expected zero launch attempts on unknown candidate, got %d", len(fl.calls))
	}
}

func TestLaunchWithFallback_EmptyCandidateList(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor(nil)

	_, err := launchWithFallbackForTest(fl, nil)
	if err == nil {
		t.Fatal("expected validation error on empty candidate list")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	_, err = launchWithFallbackForTest(fl, []string{"", "   "})
	if err == nil {
		t.Fatal("expected validation error on whitespace candidates")
	}
}

func TestLaunchWithFallback_DedupesCandidates(t *testing.T) {
	fastPolling(t)
	fl := fakeLauncherFor([]scriptedOutcome{
		{target: "chrome", succeed: true},
	})

	if _, err := launchWithFallbackForTest(fl, []string{"chrome", "chrome", " chrome "}); err != nil {
		t.Fatalf("LaunchWithFallback returned error: %v", err)
	}
	if len(fl.calls) != 1 {
		t.Errorf("expected dedup to reduce to 1 attempt, got %d", len(fl.calls))
	}
}

func TestIsRecoverable(t *testing.T) {
	recoverable := []LaunchFailureReason{
		ReasonStartupTimeout,
		ReasonProcessExited,
		ReasonBinaryMissing,
		ReasonCDPConnectFail,
		ReasonHealthCheckTimeout,
	}
	for _, r := range recoverable {
		if !IsRecoverable(r) {
			t.Errorf("expected %q to be recoverable", string(r))
		}
	}
	if IsRecoverable(ReasonUnknown) {
		t.Errorf("ReasonUnknown must not be recoverable")
	}
	if IsRecoverable(LaunchFailureReason("not_a_reason")) {
		t.Errorf("arbitrary string must not be recoverable")
	}
}

func TestDedupCandidates(t *testing.T) {
	got := dedupCandidates([]string{"a", " a", "b", "", "c", "b"})
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("dedupCandidates = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ = errors.New
