package strategy_test

import (
	"testing"

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
