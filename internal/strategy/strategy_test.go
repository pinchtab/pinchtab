package strategy_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/strategy"

	// Register strategies via init()
	_ "github.com/pinchtab/pinchtab/internal/strategy/alwayson"
	_ "github.com/pinchtab/pinchtab/internal/strategy/autorestart"
	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit"
	_ "github.com/pinchtab/pinchtab/internal/strategy/noinstance"
	_ "github.com/pinchtab/pinchtab/internal/strategy/simple"
)

func TestRegistry_ExplicitRegistered(t *testing.T) {
	s, err := strategy.New("explicit")
	if err != nil {
		t.Fatalf("explicit strategy not registered: %v", err)
	}
	if s.Name() != "explicit" {
		t.Errorf("expected name 'explicit', got %q", s.Name())
	}
}

func TestRegistry_SimpleRegistered(t *testing.T) {
	s, err := strategy.New("simple")
	if err != nil {
		t.Fatalf("simple strategy not registered: %v", err)
	}
	if s.Name() != "simple" {
		t.Errorf("expected name 'simple', got %q", s.Name())
	}
}

func TestRegistry_SimpleAutorestartRegistered(t *testing.T) {
	s, err := strategy.New("simple-autorestart")
	if err != nil {
		t.Fatalf("simple-autorestart strategy not registered: %v", err)
	}
	if s.Name() != "simple-autorestart" {
		t.Errorf("expected name 'simple-autorestart', got %q", s.Name())
	}
}

func TestRegistry_AlwaysOnRegistered(t *testing.T) {
	s, err := strategy.New("always-on")
	if err != nil {
		t.Fatalf("always-on strategy not registered: %v", err)
	}
	if s.Name() != "always-on" {
		t.Errorf("expected name 'always-on', got %q", s.Name())
	}
}

func TestRegistry_UnknownStrategy(t *testing.T) {
	_, err := strategy.New("nonexistent")
	if err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestRegistry_Names(t *testing.T) {
	names := strategy.Names()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["explicit"] {
		t.Error("explicit not in names")
	}
	if !found["simple"] {
		t.Error("simple not in names")
	}
	if !found["simple-autorestart"] {
		t.Error("simple-autorestart not in names")
	}
	if !found["always-on"] {
		t.Error("always-on not in names")
	}
}

type mockRunner struct{}

func (m *mockRunner) Run(_ context.Context, _ string, _ []string, _ []string, _ io.Writer, _ io.Writer) (orchestrator.Cmd, error) {
	return nil, nil
}
func (m *mockRunner) InspectPort(_ string) orchestrator.PortInspection {
	return orchestrator.PortInspection{Available: true}
}

func TestCacheRoutes_RegisteredAcrossStrategies(t *testing.T) {
	strategies := []string{"simple", "explicit", "no-instance", "simple-autorestart"}
	cacheRoutes := []struct {
		method string
		path   string
		route  string
	}{
		{"POST", "/cache/clear", "POST /cache/clear"},
		{"GET", "/cache/status", "GET /cache/status"},
	}

	for _, name := range strategies {
		t.Run(name, func(t *testing.T) {
			s, err := strategy.New(name)
			if err != nil {
				t.Fatalf("strategy.New(%q): %v", name, err)
			}

			orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{})
			orch.ApplyRuntimeConfig(&config.RuntimeConfig{})

			if oa, ok := s.(strategy.OrchestratorAware); ok {
				oa.SetOrchestrator(orch)
			}

			mux := http.NewServeMux()
			s.RegisterRoutes(mux)

			for _, rt := range cacheRoutes {
				req := httptest.NewRequest(rt.method, rt.path, nil)
				_, pattern := mux.Handler(req)
				if pattern != rt.route {
					t.Errorf("strategy %q: expected route %q for %s %s, got %q", name, rt.route, rt.method, rt.path, pattern)
				}
			}
		})
	}
}

func TestOrchestratorAware_AllStrategies(t *testing.T) {
	for _, name := range []string{"explicit", "simple", "simple-autorestart", "always-on"} {
		s, err := strategy.New(name)
		if err != nil {
			t.Fatalf("strategy %q not registered: %v", name, err)
		}
		if _, ok := s.(strategy.OrchestratorAware); !ok {
			t.Errorf("strategy %q does not implement OrchestratorAware", name)
		}
	}
}
