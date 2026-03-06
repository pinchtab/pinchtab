package strategy_test

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/strategy"

	// Register strategies via init()
	_ "github.com/pinchtab/pinchtab/internal/strategy/autorestart"
	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit"
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
}

func TestOrchestratorAware_AllStrategies(t *testing.T) {
	for _, name := range []string{"explicit", "simple", "simple-autorestart"} {
		s, err := strategy.New(name)
		if err != nil {
			t.Fatalf("strategy %q not registered: %v", name, err)
		}
		if _, ok := s.(strategy.OrchestratorAware); !ok {
			t.Errorf("strategy %q does not implement OrchestratorAware", name)
		}
	}
}
