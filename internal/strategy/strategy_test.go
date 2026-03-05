package strategy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/strategy/session"
	"github.com/pinchtab/pinchtab/internal/strategy/simple"

	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit"
)

// mockLauncher satisfies instance.InstanceLauncher for tests.
type mockLauncher struct{}

func (m *mockLauncher) Launch(string, string, bool) (*bridgepkg.Instance, error) { return nil, nil }
func (m *mockLauncher) Stop(string) error                                        { return nil }

func TestRegistry(t *testing.T) {
	names := strategy.Names()

	// Explicit is registered via init().
	found := false
	for _, n := range names {
		if n == "explicit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("explicit strategy not registered")
	}
}

func TestNewStrategy(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"explicit", false},
		{"unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := strategy.New(tt.name)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if s.Name() != tt.name {
				t.Errorf("got name %q, want %q", s.Name(), tt.name)
			}
		})
	}
}

func TestMustRegister_Duplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()

	// This should panic because "explicit" is already registered
	strategy.MustRegister("explicit", func() strategy.Strategy {
		return nil
	})
}

func TestRegister_Duplicate(t *testing.T) {
	err := strategy.Register("explicit", func() strategy.Strategy {
		return nil
	})
	if err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestExplicitStrategyLifecycle(t *testing.T) {
	s, err := strategy.New("explicit")
	if err != nil {
		t.Fatalf("failed to create strategy: %v", err)
	}

	if err := s.Init(nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// newTestManager creates a Manager with a no-op launcher for lifecycle tests.
func newTestManager() *instance.Manager {
	return instance.NewManager(&mockLauncher{}, instance.NewBridgeClient(), nil)
}

func TestSimpleStrategyLifecycle(t *testing.T) {
	s := simple.New(newTestManager())

	if err := s.Init(nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestSessionStrategyLifecycle(t *testing.T) {
	s := session.New(newTestManager())

	if err := s.Init(nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestStrategySwitch(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager()

	// Start with explicit.
	s1, _ := strategy.New("explicit")
	_ = s1.Init(nil)
	mux1 := http.NewServeMux()
	s1.RegisterRoutes(mux1)
	_ = s1.Start(ctx)
	srv1 := httptest.NewServer(mux1)
	defer srv1.Close()
	_ = s1.Stop()

	// Switch to session.
	s2 := session.New(mgr)
	_ = s2.Init(nil)
	mux2 := http.NewServeMux()
	s2.RegisterRoutes(mux2)
	_ = s2.Start(ctx)
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	_ = s2.Stop()

	// Switch to simple.
	s3 := simple.New(mgr)
	_ = s3.Init(nil)
	mux3 := http.NewServeMux()
	s3.RegisterRoutes(mux3)
	_ = s3.Start(ctx)
	srv3 := httptest.NewServer(mux3)
	defer srv3.Close()
	_ = s3.Stop()
}

func TestSessionStrategyCleanShutdown(t *testing.T) {
	s := session.New(newTestManager())
	_ = s.Init(nil)

	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	cancel()

	done := make(chan struct{})
	go func() {
		_ = s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung")
	}
}
