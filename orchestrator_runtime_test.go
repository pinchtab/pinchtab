package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstanceIsActiveStoppedButAlivePID(t *testing.T) {
	inst := &Instance{Status: "stopped", PID: os.Getpid()}
	if !instanceIsActive(inst) {
		t.Fatal("expected instance to be active when pid is alive")
	}
}

func TestLaunchRejectsStoppedButActiveInstance(t *testing.T) {
	o := &Orchestrator{
		instances: map[string]*Instance{
			"p1-9911": {
				ID:     "p1-9911",
				Name:   "p1",
				Port:   "9911",
				Status: "stopped",
				PID:    os.Getpid(),
			},
		},
		baseDir: t.TempDir(),
		binary:  "pinchtab",
		client:  &http.Client{Timeout: 10 * time.Millisecond},
	}

	_, err := o.Launch("p1", "9911", true)
	if err == nil {
		t.Fatal("expected launch to fail for active profile")
	}
	if !strings.Contains(err.Error(), "already has an active instance") && !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListReconcilesStoppedButAliveToRunning(t *testing.T) {
	o := &Orchestrator{
		instances: map[string]*Instance{
			"p1-9912": {
				ID:     "p1-9912",
				Name:   "p1",
				Port:   "9912",
				Status: "stopped",
				PID:    os.Getpid(),
				URL:    "http://127.0.0.1:1",
			},
		},
		client: &http.Client{Timeout: 10 * time.Millisecond},
	}

	list := o.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list))
	}
	if list[0].Status != "running" {
		t.Fatalf("expected reconciled status running, got %q", list[0].Status)
	}
}

func TestMergeEnvWithOverridesReplacesExistingValues(t *testing.T) {
	base := []string{
		"BRIDGE_PORT=9870",
		"BRIDGE_HEADLESS=true",
		"PATH=/usr/bin",
	}
	merged := mergeEnvWithOverrides(base, map[string]string{
		"BRIDGE_PORT":     "9868",
		"BRIDGE_HEADLESS": "false",
		"BRIDGE_PROFILE":  "/tmp/profile",
	})
	joined := strings.Join(merged, "\n")

	if strings.Contains(joined, "BRIDGE_PORT=9870") {
		t.Fatalf("expected old BRIDGE_PORT to be removed, got %v", merged)
	}
	if !strings.Contains(joined, "BRIDGE_PORT=9868") {
		t.Fatalf("expected overridden BRIDGE_PORT, got %v", merged)
	}
	if !strings.Contains(joined, "BRIDGE_HEADLESS=false") {
		t.Fatalf("expected overridden BRIDGE_HEADLESS, got %v", merged)
	}
	if !strings.Contains(joined, "BRIDGE_PROFILE=/tmp/profile") {
		t.Fatalf("expected BRIDGE_PROFILE override, got %v", merged)
	}
}

func TestIsInstanceHealthyStatus(t *testing.T) {
	tests := []struct {
		code int
		ok   bool
	}{
		{code: 200, ok: true},
		{code: 401, ok: true},
		{code: 404, ok: true},
		{code: 500, ok: false},
		{code: 503, ok: false},
	}
	for _, tt := range tests {
		if got := isInstanceHealthyStatus(tt.code); got != tt.ok {
			t.Fatalf("code %d: got %v, want %v", tt.code, got, tt.ok)
		}
	}
}

func TestIsStartingConflict(t *testing.T) {
	if !isStartingConflict(fmt.Errorf(`profile "x" already has an active instance (starting)`)) {
		t.Fatal("expected starting conflict to match")
	}
	if isStartingConflict(fmt.Errorf(`profile "x" already has an active instance (running)`)) {
		t.Fatal("did not expect running conflict to match")
	}
}

func TestFindProfileInstancePrefersRunning(t *testing.T) {
	now := time.Now()
	inst, ok := findProfileInstance([]Instance{
		{Name: "alpha", Status: "error", Port: "9868", StartedAt: now.Add(-time.Minute)},
		{Name: "alpha", Status: "running", Port: "9869", StartedAt: now},
		{Name: "beta", Status: "running", Port: "9870", StartedAt: now},
	}, "alpha")
	if !ok {
		t.Fatal("expected instance for alpha")
	}
	if inst.Status != "running" || inst.Port != "9869" {
		t.Fatalf("unexpected selected instance: %+v", inst)
	}
}

func TestFindProfileInstanceMissing(t *testing.T) {
	_, ok := findProfileInstance([]Instance{{Name: "a", Status: "stopped"}}, "nope")
	if ok {
		t.Fatal("expected no instance")
	}
}

func TestInstanceBaseURLs(t *testing.T) {
	got := instanceBaseURLs("9869")
	if len(got) != 3 {
		t.Fatalf("expected 3 base URLs, got %d", len(got))
	}
	if got[0] != "http://127.0.0.1:9869" {
		t.Fatalf("unexpected IPv4 URL: %s", got[0])
	}
	if got[1] != "http://[::1]:9869" {
		t.Fatalf("unexpected IPv6 URL: %s", got[1])
	}
	if got[2] != "http://localhost:9869" {
		t.Fatalf("unexpected localhost URL: %s", got[2])
	}
}

func TestTailLogLine(t *testing.T) {
	got := tailLogLine("\nfirst\n\nsecond\n")
	if got != "second" {
		t.Fatalf("unexpected tail line: %q", got)
	}
}

func TestNewOrchestratorPrefersCurrentExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		t.Skip("no executable path available")
	}

	base := filepath.Join(t.TempDir(), "profiles")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}

	o := NewOrchestrator(base)
	if o.binary != exe {
		t.Fatalf("expected binary %q, got %q", exe, o.binary)
	}
}
