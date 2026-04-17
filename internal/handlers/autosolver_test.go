package handlers

import (
	"context"
	"testing"

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

	calls := 0
	h.autoSolverRunner = func(_ context.Context, tabID string) error {
		calls++
		if tabID != "tab1" {
			t.Fatalf("runner tabID = %q, want tab1", tabID)
		}
		return nil
	}

	h.maybeAutoSolve(context.Background(), "tab1", autoSolverTriggerNavigate)
	if calls != 1 {
		t.Fatalf("autoSolverRunner calls = %d, want 1", calls)
	}

	h.maybeAutoSolve(context.Background(), "", autoSolverTriggerNavigate)
	if calls != 1 {
		t.Fatalf("autoSolverRunner calls with empty tab id = %d, want unchanged", calls)
	}

	h.Config.AutoSolver.TriggerOnNavigate = false
	h.maybeAutoSolve(context.Background(), "tab1", autoSolverTriggerNavigate)
	if calls != 1 {
		t.Fatalf("autoSolverRunner calls with navigate trigger disabled = %d, want unchanged", calls)
	}
}
