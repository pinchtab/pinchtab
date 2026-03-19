package noinstance

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/strategy"
)

func TestNoInstanceStrategyRegistered(t *testing.T) {
	s, err := strategy.New("no-instance")
	if err != nil {
		t.Fatalf("strategy.New(%q) error = %v", "no-instance", err)
	}
	if s.Name() != "no-instance" {
		t.Fatalf("Name() = %q, want %q", s.Name(), "no-instance")
	}
}
