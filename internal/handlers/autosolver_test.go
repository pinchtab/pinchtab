package handlers

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/autosolver/catalog"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestLLMProviderForAutoSolver(t *testing.T) {
	// LLM provider configured → instantiated (wires the llmFallback switch).
	h := &Handlers{Config: &config.RuntimeConfig{
		AutoSolver: config.AutoSolverConfig{LLMProvider: "openai"},
	}}
	if h.llmProviderForAutoSolver() == nil {
		t.Error("expected non-nil LLM provider when LLMProvider is set")
	}

	// No provider configured → nil (LLM branch stays inert, as before).
	h = &Handlers{Config: &config.RuntimeConfig{}}
	if h.llmProviderForAutoSolver() != nil {
		t.Error("expected nil LLM provider when LLMProvider is empty")
	}

	// nil Config → nil.
	if (&Handlers{}).llmProviderForAutoSolver() != nil {
		t.Error("expected nil LLM provider with nil Config")
	}
}

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

// The catalog is only a single owner if what actually registers stays inside it.
// A new solver wired into buildAutoSolver but not added to the catalog would
// leave config validation rejecting a name the product really accepts, and this
// is the link that fails when that happens.
func TestRegisteredSolversAreAllKnownToTheCatalog(t *testing.T) {
	h := &Handlers{Config: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
		CapsolverKey:  "test-capsolver-key",
		TwoCaptchaKey: "test-twocaptcha-key",
	}}}

	// The normalised config, exactly as both production call sites build it: the keys
	// travel to the registry through cfg.APIKeys now, so handing buildAutoSolver a bare
	// DefaultConfig would register nothing gated and assert against a config nobody uses.
	as := h.buildAutoSolver(h.normalizedAutoSolverConfig(), true)
	registered := as.Registry().Names()
	if len(registered) == 0 {
		t.Fatal("no solvers registered — this guard is checking nothing")
	}

	for _, name := range registered {
		if !catalog.IsKnown(name) {
			t.Errorf("solver %q registers but config validation rejects it (known: %v)", name, catalog.Names())
		}
	}

	for _, gated := range autosolver.KeyGatedSolvers() {
		if slices.Contains(registered, gated.Name) == gated.Planned {
			t.Errorf("%q registered = %v with its key set, want %v (planned = %v; registered: %v)", gated.Name, !gated.Planned, !gated.Planned, gated.Planned, registered)
		}
	}
}

// The handler's only remaining per-solver knowledge is DATA: which runtime field holds
// each gated solver's key. The rule that consumes it lives in the catalog, so this pins
// the seam between them — every gated solver's key must actually arrive, or availability
// silently answers "keyless" for a solver the operator configured.
func TestEveryGatedSolverKeyReachesTheNormalisedConfig(t *testing.T) {
	h := &Handlers{Config: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
		CapsolverKey:  "cap-key",
		TwoCaptchaKey: "two-key",
	}}}

	cfg := h.normalizedAutoSolverConfig()
	gated := autosolver.KeyGatedSolvers()
	if len(gated) == 0 {
		t.Fatal("no key-gated solvers; this seam test would check nothing")
	}
	for _, solver := range gated {
		if cfg.APIKey(solver.Name) == "" {
			t.Errorf("%s is key-gated but its key never reaches the config the catalog reads; add it to autoSolverAPIKeys beside %s", solver.Name, solver.ConfigKey)
		}
		if catalog.IsAvailable(solver.Name, cfg) == solver.Planned {
			t.Errorf("%s available = %v with its key configured, want %v (planned = %v)", solver.Name, solver.Planned, !solver.Planned, solver.Planned)
		}
	}

	keyless := (&Handlers{Config: &config.RuntimeConfig{}}).normalizedAutoSolverConfig()
	for _, solver := range gated {
		if catalog.IsAvailable(solver.Name, keyless) {
			t.Errorf("%s is available with no key configured", solver.Name)
		}
	}
}

// availableAutoSolverNames is the list the API prints and guards on, and it must follow
// the catalog rather than re-deriving availability. Both directions, because the defect
// this card fixes was a configured solver reported as unknown.
func TestAvailableNamesFollowTheConfiguredKeys(t *testing.T) {
	for _, solver := range autosolver.KeyGatedSolvers() {
		t.Run(solver.Name, func(t *testing.T) {
			keyless := &Handlers{Config: &config.RuntimeConfig{}}
			if slices.Contains(keyless.availableAutoSolverNames(), solver.Name) {
				t.Errorf("%s is listed available with no key set", solver.Name)
			}
			if keyless.isAvailableAutoSolver(solver.Name) {
				t.Errorf("isAvailableAutoSolver(%s) with no key set", solver.Name)
			}

			keyed := &Handlers{Config: &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
				CapsolverKey:  "k",
				TwoCaptchaKey: "k",
				Solvers:       []string{solver.Name},
			}}}
			if slices.Contains(keyed.availableAutoSolverNames(), solver.Name) == solver.Planned {
				t.Errorf("%s listed = %v with its key set, want %v (planned = %v; list %v)", solver.Name, solver.Planned, !solver.Planned, solver.Planned, keyed.availableAutoSolverNames())
			}
		})
	}
}
