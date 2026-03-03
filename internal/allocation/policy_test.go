package allocation_test

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/allocation"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

func candidates(ids ...string) []bridge.Instance {
	out := make([]bridge.Instance, len(ids))
	for i, id := range ids {
		out[i] = bridge.Instance{ID: id, Status: "running"}
	}
	return out
}

func TestFCFS_SelectsFirst(t *testing.T) {
	p := &allocation.FCFS{}
	got, err := p.Select(candidates("a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "a" {
		t.Errorf("expected a, got %s", got.ID)
	}
}

func TestFCFS_EmptyReturnsError(t *testing.T) {
	p := &allocation.FCFS{}
	_, err := p.Select(nil)
	if err != allocation.ErrNoCandidates {
		t.Errorf("expected ErrNoCandidates, got %v", err)
	}
}

func TestRoundRobin_Cycles(t *testing.T) {
	p := allocation.NewRoundRobin()
	c := candidates("a", "b", "c")

	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, want := range expected {
		got, err := p.Select(c)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != want {
			t.Errorf("call %d: expected %s, got %s", i, want, got.ID)
		}
	}
}

func TestRoundRobin_EmptyReturnsError(t *testing.T) {
	p := allocation.NewRoundRobin()
	_, err := p.Select(nil)
	if err != allocation.ErrNoCandidates {
		t.Errorf("expected ErrNoCandidates, got %v", err)
	}
}

func TestRandom_SelectsFromCandidates(t *testing.T) {
	p := &allocation.Random{}
	c := candidates("a", "b", "c")

	// Run enough times to verify it doesn't panic and returns valid candidates.
	seen := map[string]bool{}
	for range 100 {
		got, err := p.Select(c)
		if err != nil {
			t.Fatal(err)
		}
		seen[got.ID] = true
	}
	// With 100 tries and 3 candidates, we should see at least 2.
	if len(seen) < 2 {
		t.Errorf("random policy only selected %d unique candidates in 100 tries", len(seen))
	}
}

func TestRandom_EmptyReturnsError(t *testing.T) {
	p := &allocation.Random{}
	_, err := p.Select(nil)
	if err != allocation.ErrNoCandidates {
		t.Errorf("expected ErrNoCandidates, got %v", err)
	}
}

func TestNew_KnownPolicies(t *testing.T) {
	tests := []struct {
		name     string
		wantName string
	}{
		{"fcfs", "fcfs"},
		{"", "fcfs"},
		{"round_robin", "round_robin"},
		{"random", "random"},
	}
	for _, tt := range tests {
		p, err := allocation.New(tt.name)
		if err != nil {
			t.Errorf("New(%q): %v", tt.name, err)
			continue
		}
		if p.Name() != tt.wantName {
			t.Errorf("New(%q).Name() = %q, want %q", tt.name, p.Name(), tt.wantName)
		}
	}
}

func TestNew_UnknownPolicy(t *testing.T) {
	_, err := allocation.New("does_not_exist")
	if err == nil {
		t.Error("expected error for unknown policy")
	}
}
