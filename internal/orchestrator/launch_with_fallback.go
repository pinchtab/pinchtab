package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ErrUnknownBrowser disambiguates 400 (unknown target) from 502 (exhaustion) for handlers.
var ErrUnknownBrowser = errors.New("unknown browser target")

// UnknownBrowserError satisfies errors.Is(ErrUnknownBrowser).
type UnknownBrowserError struct {
	Target string
	Err    error
}

func (e *UnknownBrowserError) Error() string {
	return fmt.Sprintf("launch with fallback: resolve target %q: %v", e.Target, e.Err)
}

func (e *UnknownBrowserError) Unwrap() error { return e.Err }

func (e *UnknownBrowserError) Is(target error) bool {
	return target == ErrUnknownBrowser
}

type FallbackAttempt struct {
	Target string              `json:"target"`
	Reason LaunchFailureReason `json:"reason"`
}

// FallbackExhaustedError maps to HTTP 502 browser_target_unavailable.
type FallbackExhaustedError struct {
	Attempts []FallbackAttempt
	Cause    error `json:"-"`
}

func (e *FallbackExhaustedError) Error() string {
	parts := make([]string, 0, len(e.Attempts))
	for _, a := range e.Attempts {
		parts = append(parts, fmt.Sprintf("%s:%s", a.Target, string(a.Reason)))
	}
	msg := fmt.Sprintf("all browser targets failed: [%s]", strings.Join(parts, ", "))
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *FallbackExhaustedError) Unwrap() error { return e.Cause }

// launchWithFallbackTimeout covers the monitor's instanceStartupTimeout plus a margin.
const launchWithFallbackTimeout = 60 * time.Second

var launchOutcomePollInterval = 50 * time.Millisecond

// ResolvedLaunchTarget is the target/provider pair the planner passes into a
// launch attempt.
type ResolvedLaunchTarget struct {
	Name     string
	Provider string
}

// LaunchOutcome is the terminal state observed for a launched attempt.
type LaunchOutcome struct {
	Status string
	Reason LaunchFailureReason
}

// PlannedLaunch is the planner result before orchestrator-specific instance
// storage metadata has been applied.
type PlannedLaunch struct {
	Instance       *bridge.Instance
	FallbackFrom   string
	FallbackReason LaunchFailureReason
}

// Launcher is the dependency boundary for fallback planning. Implementations
// provide target resolution, launch execution, outcome polling, and teardown.
type Launcher interface {
	ResolveTarget(candidate string) (ResolvedLaunchTarget, error)
	Launch(name, port string, headless bool, opts LaunchOptions) (*bridge.Instance, error)
	WaitForLaunchOutcome(instanceID string, timeout time.Duration) (LaunchOutcome, error)
	TearDownFailedAttempt(instanceID string)
}

// LaunchPlanner contains fallback policy without depending on Orchestrator
// internals. It is intentionally small so policy tests can use a fake Launcher.
type LaunchPlanner struct {
	Launcher Launcher
	Timeout  time.Duration
}

type fallbackAttempt struct {
	target string
	reason LaunchFailureReason
}

func (a fallbackAttempt) String() string {
	return fmt.Sprintf("%s:%s", a.target, string(a.reason))
}

// LaunchWithFallback tries candidates in order, tearing down recoverable failures before moving on.
// Returns *UnknownBrowserError for invalid names, *FallbackExhaustedError when all candidates fail.
func (p LaunchPlanner) LaunchWithFallback(
	name, port string, headless bool,
	primaryAndFallbacks []string,
	opts LaunchOptions,
) (*PlannedLaunch, error) {
	if p.Launcher == nil {
		return nil, fmt.Errorf("launch with fallback: launcher is required")
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = launchWithFallbackTimeout
	}
	candidates := dedupCandidates(primaryAndFallbacks)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("launch with fallback: at least one browser target candidate is required")
	}

	var (
		attempts        []fallbackAttempt
		firstFailTarget string
		firstFailReason LaunchFailureReason
	)

	for i, candidate := range candidates {
		resolved, err := p.Launcher.ResolveTarget(candidate)
		if err != nil {
			return nil, &UnknownBrowserError{Target: candidate, Err: err}
		}

		attemptOpts := opts
		attemptOpts.ResolvedTarget = resolved.Name
		attemptOpts.BrowserProvider = resolved.Provider

		inst, launchErr := p.Launcher.Launch(name, port, headless, attemptOpts)
		if launchErr != nil {
			reason := ClassifyLaunchFailure(launchErr)
			attempts = append(attempts, fallbackAttempt{target: candidate, reason: reason})
			if i == 0 {
				firstFailTarget = candidate
				firstFailReason = reason
			}
			if !IsRecoverable(reason) {
				return nil, fallbackExhaustedError(attempts, fmt.Errorf(
					"launch with fallback: candidate %q failed non-recoverably (%s): %w",
					candidate, string(reason), launchErr,
				))
			}
			continue
		}
		if inst == nil {
			reason := ReasonUnknown
			attempts = append(attempts, fallbackAttempt{target: candidate, reason: reason})
			return nil, fallbackExhaustedError(attempts, fmt.Errorf(
				"launch with fallback: candidate %q returned nil instance",
				candidate,
			))
		}

		outcome, waitErr := p.Launcher.WaitForLaunchOutcome(inst.ID, timeout)
		if waitErr != nil {
			reason := ClassifyLaunchFailure(waitErr)
			attempts = append(attempts, fallbackAttempt{target: candidate, reason: reason})
			p.Launcher.TearDownFailedAttempt(inst.ID)
			return nil, fallbackExhaustedError(attempts, fmt.Errorf(
				"launch with fallback: wait for %q: %w", candidate, waitErr,
			))
		}

		if outcome.Status == "running" {
			result := &PlannedLaunch{Instance: inst}
			if i > 0 {
				result.FallbackFrom = firstFailTarget
				result.FallbackReason = firstFailReason
			}
			return result, nil
		}

		attempts = append(attempts, fallbackAttempt{target: candidate, reason: outcome.Reason})
		if i == 0 {
			firstFailTarget = candidate
			firstFailReason = outcome.Reason
		}

		if !IsRecoverable(outcome.Reason) {
			p.Launcher.TearDownFailedAttempt(inst.ID)
			return nil, fallbackExhaustedError(attempts, fmt.Errorf(
				"launch with fallback: candidate %q failed non-recoverably (%s)",
				candidate, string(outcome.Reason),
			))
		}

		p.Launcher.TearDownFailedAttempt(inst.ID)
	}

	return nil, fallbackExhaustedError(attempts, nil)
}

// LaunchWithFallback tries candidates in order, tearing down recoverable failures before moving on.
// Returns *UnknownBrowserError for invalid names, *FallbackExhaustedError when all candidates fail.
func (o *Orchestrator) LaunchWithFallback(
	name, port string, headless bool,
	primaryAndFallbacks []string,
	opts LaunchOptions,
) (*bridge.Instance, error) {
	planner := LaunchPlanner{
		Launcher: o.fallbackLaunchPlannerLauncher(),
		Timeout:  launchWithFallbackTimeout,
	}
	planned, err := planner.LaunchWithFallback(name, port, headless, primaryAndFallbacks, opts)
	if err != nil {
		return nil, err
	}
	return o.snapshotWithFallbackMetadata(planned), nil
}

func (o *Orchestrator) fallbackLaunchPlannerLauncher() Launcher {
	if o != nil && o.fallbackLauncher != nil {
		return o.fallbackLauncher
	}
	return orchestratorFallbackLauncher{orch: o}
}

type orchestratorFallbackLauncher struct {
	orch *Orchestrator
}

func (l orchestratorFallbackLauncher) ResolveTarget(candidate string) (ResolvedLaunchTarget, error) {
	resolved, err := config.ResolveExplicitBrowserTarget(l.orch.runtimeCfg, candidate)
	if err != nil {
		return ResolvedLaunchTarget{}, err
	}
	return ResolvedLaunchTarget{Name: resolved.Name, Provider: resolved.Provider}, nil
}

func (l orchestratorFallbackLauncher) Launch(name, port string, headless bool, opts LaunchOptions) (*bridge.Instance, error) {
	return l.orch.LaunchWithOptions(name, port, headless, opts)
}

func (l orchestratorFallbackLauncher) WaitForLaunchOutcome(instanceID string, timeout time.Duration) (LaunchOutcome, error) {
	status, reason, err := l.orch.waitForLaunchOutcome(instanceID, timeout)
	return LaunchOutcome{Status: status, Reason: reason}, err
}

func (l orchestratorFallbackLauncher) TearDownFailedAttempt(instanceID string) {
	l.orch.tearDownFailedAttempt(instanceID)
}

func (o *Orchestrator) snapshotWithFallbackMetadata(planned *PlannedLaunch) *bridge.Instance {
	if planned == nil || planned.Instance == nil {
		return nil
	}
	if planned.FallbackFrom == "" {
		return planned.Instance
	}

	reason := string(planned.FallbackReason)
	o.mu.Lock()
	if internal, ok := o.instances[planned.Instance.ID]; ok {
		internal.FallbackFrom = planned.FallbackFrom
		internal.FallbackReason = reason
		snapshot := internal.Instance
		o.mu.Unlock()
		o.syncInstanceToManager(&snapshot)
		return &snapshot
	}
	o.mu.Unlock()

	snapshot := *planned.Instance
	snapshot.FallbackFrom = planned.FallbackFrom
	snapshot.FallbackReason = reason
	return &snapshot
}

func fallbackExhaustedError(attempts []fallbackAttempt, cause error) *FallbackExhaustedError {
	exhausted := &FallbackExhaustedError{
		Attempts: make([]FallbackAttempt, 0, len(attempts)),
		Cause:    cause,
	}
	for _, a := range attempts {
		exhausted.Attempts = append(exhausted.Attempts, FallbackAttempt{Target: a.target, Reason: a.reason})
	}
	return exhausted
}

// waitForLaunchOutcome polls until the instance reaches running/error or the timeout fires.
func (o *Orchestrator) waitForLaunchOutcome(instanceID string, timeout time.Duration) (string, LaunchFailureReason, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(launchOutcomePollInterval)
	defer ticker.Stop()

	for {
		o.mu.RLock()
		inst, ok := o.instances[instanceID]
		var status string
		var reason LaunchFailureReason
		if ok {
			status = inst.Status
			reason = inst.lastFailureReason
		}
		o.mu.RUnlock()

		if !ok {
			return "", ReasonUnknown, fmt.Errorf("instance %q disappeared from orchestrator map", instanceID)
		}

		switch status {
		case "running":
			return "running", ReasonUnknown, nil
		case "error":
			return "error", reason, nil
		}

		select {
		case <-ctx.Done():
			return "", ReasonUnknown, ctx.Err()
		case <-ticker.C:
		}
	}
}

// tearDownFailedAttempt logs but does not propagate errors so teardown never blocks fallback.
func (o *Orchestrator) tearDownFailedAttempt(instanceID string) {
	inst := o.detachFailedAttempt(instanceID)
	if inst == nil {
		return
	}
	go o.stopDetachedFailedAttempt(instanceID, inst)
}

func (o *Orchestrator) detachFailedAttempt(instanceID string) *InstanceInternal {
	o.mu.Lock()
	inst, ok := o.instances[instanceID]
	if !ok {
		o.mu.Unlock()
		return nil
	}
	inst.Status = "stopping"
	delete(o.instances, instanceID)
	o.mu.Unlock()

	o.removeInstanceFromManager(instanceID)
	o.releaseInstancePorts(instanceID, inst)
	return inst
}

func (o *Orchestrator) stopDetachedFailedAttempt(instanceID string, inst *InstanceInternal) {
	if inst == nil {
		return
	}
	defer func() {
		o.cleanupDetachedFailedAttemptProfile(inst.ProfileName)
	}()

	if inst.cmd == nil {
		if inst.AttachType == "bridge" {
			o.requestDetachedShutdown(inst)
		}
		return
	}

	o.requestDetachedShutdown(inst)
	pid := inst.cmd.PID()
	if pid > 0 {
		if waitForProcessExit(pid, 5*time.Second) {
			return
		}

		if err := killProcessGroup(pid, sigTERM); err != nil {
			slog.Warn("failed to send SIGTERM to failed fallback attempt", "id", instanceID, "pid", pid, "err", err)
		}
		if waitForProcessExit(pid, 3*time.Second) {
			return
		}

		if err := killProcessGroup(pid, sigKILL); err != nil {
			slog.Warn("failed to send SIGKILL to failed fallback attempt", "id", instanceID, "pid", pid, "err", err)
		}
	}

	inst.cmd.Cancel()
	if pid > 0 && !waitForProcessExit(pid, 2*time.Second) {
		slog.Warn("failed fallback attempt still running after teardown", "id", instanceID, "pid", pid)
	}
}

func (o *Orchestrator) requestDetachedShutdown(inst *InstanceInternal) {
	reqCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	targetURL, targetErr := o.instancePathURL(inst, "/shutdown", "")
	if targetErr != nil {
		return
	}
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL.String(), nil)
	o.applyInstanceAuth(req, inst)
	if resp, err := o.client.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}

func (o *Orchestrator) cleanupDetachedFailedAttemptProfile(profileName string) {
	o.mu.RLock()
	for _, inst := range o.instances {
		if inst != nil && inst.ProfileName == profileName && instanceIsActive(inst) {
			o.mu.RUnlock()
			return
		}
	}
	o.mu.RUnlock()
	o.cleanupStoppedProfile(profileName)
}

// dedupCandidates trims whitespace, drops empty/duplicate entries, preserves order.
func dedupCandidates(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
