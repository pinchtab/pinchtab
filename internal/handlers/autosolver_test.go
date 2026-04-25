package handlers

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestShouldAutoSolve(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.RuntimeConfig
		trigger string
		want    bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			trigger: autoSolverTriggerNavigate,
			want:    false,
		},
		{
			name: "disabled autosolver",
			cfg: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
				Enabled:           false,
				AutoTrigger:       true,
				TriggerOnNavigate: true,
				TriggerOnAction:   true,
			}},
			trigger: autoSolverTriggerNavigate,
			want:    false,
		},
		{
			name: "auto trigger disabled",
			cfg: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
				Enabled:           true,
				AutoTrigger:       false,
				TriggerOnNavigate: true,
				TriggerOnAction:   true,
			}},
			trigger: autoSolverTriggerNavigate,
			want:    false,
		},
		{
			name: "navigate trigger enabled",
			cfg: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
				Enabled:           true,
				AutoTrigger:       true,
				TriggerOnNavigate: true,
				TriggerOnAction:   false,
			}},
			trigger: autoSolverTriggerNavigate,
			want:    true,
		},
		{
			name: "action trigger disabled",
			cfg: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
				Enabled:           true,
				AutoTrigger:       true,
				TriggerOnNavigate: true,
				TriggerOnAction:   false,
			}},
			trigger: autoSolverTriggerAction,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handlers{Config: tt.cfg}
			if got := h.shouldAutoSolve(tt.trigger); got != tt.want {
				t.Fatalf("shouldAutoSolve(%q) = %v, want %v", tt.trigger, got, tt.want)
			}
		})
	}
}

func TestMaybeAutoSolve_InvokesRunnerWhenEnabled(t *testing.T) {
	h := &Handlers{
		Config: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
			Enabled:           true,
			AutoTrigger:       true,
			TriggerOnNavigate: true,
			TriggerOnAction:   true,
		}},
	}

	var calls atomic.Int64
	done := make(chan struct{}, 8)
	h.autoSolverRunner = func(_ context.Context, tabID string) error {
		calls.Add(1)
		if tabID != "tab1" {
			t.Errorf("runner tabID = %q, want tab1", tabID)
		}
		done <- struct{}{}
		return nil
	}

	waitFor := func(expected int64) bool {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if calls.Load() == expected {
				return true
			}
			time.Sleep(5 * time.Millisecond)
		}
		return false
	}

	h.maybeAutoSolve(context.Background(), "tab1", autoSolverTriggerNavigate)
	if !waitFor(1) {
		t.Fatalf("autoSolverRunner calls = %d, want 1", calls.Load())
	}
	<-done

	h.maybeAutoSolve(context.Background(), "", autoSolverTriggerNavigate)
	time.Sleep(20 * time.Millisecond) // ensure no goroutine was spawned
	if got := calls.Load(); got != 1 {
		t.Fatalf("autoSolverRunner calls with empty tab id = %d, want unchanged", got)
	}

	h.Config.AutoSolver.TriggerOnNavigate = false
	h.maybeAutoSolve(context.Background(), "tab1", autoSolverTriggerNavigate)
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("autoSolverRunner calls with navigate trigger disabled = %d, want unchanged", got)
	}
}
