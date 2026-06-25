package session

import (
	"testing"
	"time"
)

func TestStoreGetReturnsDefensiveCopy(t *testing.T) {
	s := NewStore(Config{Enabled: true, IdleTimeout: time.Hour, MaxLifetime: 24 * time.Hour})
	id, _, err := s.Create("agent", "label", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Seed grants directly (no public setter); same-package access.
	s.sessions[id].Grants = []string{"read"}

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("Get: session not found")
	}
	// Mutating the returned copy must not leak into the store.
	got.Status = "tampered"
	got.Grants[0] = "admin"

	again, _ := s.Get(id)
	if again.Status == "tampered" {
		t.Error("Get returned a live pointer: Status mutation leaked into the store")
	}
	if again.Grants[0] != "read" {
		t.Errorf("Get aliased the Grants slice: store grant = %q, want \"read\"", again.Grants[0])
	}
}

func TestStoreListReturnsDefensiveCopies(t *testing.T) {
	s := NewStore(Config{Enabled: true, IdleTimeout: time.Hour, MaxLifetime: 24 * time.Hour})
	id, _, err := s.Create("agent", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.sessions[id].Grants = []string{"read"}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	list[0].Grants[0] = "admin"

	if s.sessions[id].Grants[0] != "read" {
		t.Errorf("List aliased the Grants slice: store grant = %q, want \"read\"", s.sessions[id].Grants[0])
	}
}
