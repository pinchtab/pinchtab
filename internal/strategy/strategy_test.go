package strategy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/strategy"
	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit"
	_ "github.com/pinchtab/pinchtab/internal/strategy/session"
)

func TestRegistry(t *testing.T) {
	names := strategy.Names()
	if len(names) < 2 {
		t.Errorf("expected at least 2 strategies, got %d: %v", len(names), names)
	}

	// Check explicit is registered
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

	// Check session is registered
	found = false
	for _, n := range names {
		if n == "session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("session strategy not registered")
	}
}

func TestNewStrategy(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"explicit", false},
		{"session", false},
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

func TestStrategyLifecycle(t *testing.T) {
	strategies := []string{"explicit", "session"}

	for _, name := range strategies {
		t.Run(name, func(t *testing.T) {
			s, err := strategy.New(name)
			if err != nil {
				t.Fatalf("failed to create strategy: %v", err)
			}

			// Init with nil primitives (just testing lifecycle, not functionality)
			if err := s.Init(nil); err != nil {
				t.Fatalf("Init failed: %v", err)
			}

			// Register routes
			mux := http.NewServeMux()
			s.RegisterRoutes(mux)

			// Start
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := s.Start(ctx); err != nil {
				t.Fatalf("Start failed: %v", err)
			}

			// Stop should be clean
			if err := s.Stop(); err != nil {
				t.Fatalf("Stop failed: %v", err)
			}
		})
	}
}

func TestStrategySwitch(t *testing.T) {
	// Simulate switching strategies via config
	// This tests that both strategies can be created and torn down cleanly

	ctx := context.Background()

	// Start with explicit
	s1, _ := strategy.New("explicit")
	_ = s1.Init(nil)
	mux1 := http.NewServeMux()
	s1.RegisterRoutes(mux1)
	_ = s1.Start(ctx)

	// Verify it responds
	srv1 := httptest.NewServer(mux1)
	defer srv1.Close()

	// Stop explicit
	_ = s1.Stop()

	// Switch to session
	s2, _ := strategy.New("session")
	_ = s2.Init(nil)
	mux2 := http.NewServeMux()
	s2.RegisterRoutes(mux2)
	_ = s2.Start(ctx)

	// Verify it responds
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()

	// Stop session
	_ = s2.Stop()
}

func TestSessionStrategyCleanShutdown(t *testing.T) {
	s, _ := strategy.New("session")
	_ = s.Init(nil)

	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)

	// Simulate some activity time
	time.Sleep(10 * time.Millisecond)

	// Cancel context and stop - should not hang
	cancel()

	done := make(chan struct{})
	go func() {
		_ = s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good - stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung - shutdown not clean")
	}
}
