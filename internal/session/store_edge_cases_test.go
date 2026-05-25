package session

import (
	"testing"
	"time"
)

// Store nil-safety: all public methods on nil Store return zero values without panicking.
func TestStore_NilRevokeReturnsFalse(t *testing.T) {
	var s *Store
	if s.Revoke("any-id") {
		t.Error("nil.Revoke() expected false, got true")
	}
}

func TestStore_NilAuthenticateReturnsFalse(t *testing.T) {
	var s *Store
	sess, ok := s.Authenticate("any-token")
	if sess != nil || ok {
		t.Error("nil.Authenticate() expected (nil, false)")
	}
}

func TestStore_NilListReturnsNil(t *testing.T) {
	var s *Store
	if s.List() != nil {
		t.Error("nil.List() expected nil")
	}
}

// Empty store behavior: operations on a store with no sessions.
func TestStore_ListEmptyReturnsEmptySlice(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	sessions := s.List()
	if sessions == nil {
		t.Fatal("List() on empty store returned nil, want empty slice")
	}
	if len(sessions) != 0 {
		t.Errorf("List() on empty store len = %d, want 0", len(sessions))
	}
}

func TestStore_RevokeNonexistentReturnsFalse(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	if s.Revoke("does-not-exist") {
		t.Error("Revoke(nonexistent) expected false")
	}
}

func TestStore_AuthenticateBadTokenReturnsNil(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	sess, ok := s.Authenticate("not-a-real-token")
	if sess != nil || ok {
		t.Error("Authenticate(invalid) expected (nil, false)")
	}
}

// Create with duplicate agent ID is allowed (agent can hold multiple sessions).
func TestStore_CreateDuplicateAgentAllowed(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	id1, tok1, err := s.Create("agent-dup", "label-1")
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	id2, tok2, err := s.Create("agent-dup", "label-2")
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if id1 == id2 {
		t.Error("duplicate Create() should yield distinct session IDs")
	}
	if tok1 == tok2 {
		t.Error("duplicate Create() should yield distinct tokens")
	}
	// Both sessions should be valid
	if sess, ok := s.Authenticate(tok1); sess == nil || !ok {
		t.Error("first token should still be valid")
	}
	if sess, ok := s.Authenticate(tok2); sess == nil || !ok {
		t.Error("second token should be valid")
	}
}

// Idle timeout validation: zero and negative values are accepted and normalized.
func TestStore_CreateWithZeroIdleTimeout(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred", IdleTimeout: 0})
	_, _, err := s.Create("agent-zero", "")
	if err != nil {
		t.Fatalf("Create with IdleTimeout=0: %v", err)
	}
}

func TestStore_CreateWithNegativeIdleTimeout(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred", IdleTimeout: -time.Hour})
	_, _, err := s.Create("agent-neg", "")
	if err != nil {
		t.Fatalf("Create with IdleTimeout=-1h: %v", err)
	}
}

// Max lifetime validation: zero and negative values are accepted and normalized.
func TestStore_CreateWithZeroMaxLifetime(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred", MaxLifetime: 0})
	_, _, err := s.Create("agent-zero-ml", "")
	if err != nil {
		t.Fatalf("Create with MaxLifetime=0: %v", err)
	}
}

func TestStore_CreateWithNegativeMaxLifetime(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred", MaxLifetime: -time.Hour})
	_, _, err := s.Create("agent-neg-ml", "")
	if err != nil {
		t.Fatalf("Create with MaxLifetime=-1h: %v", err)
	}
}

// OnLifecycle with nil function is a no-op.
func TestStore_OnLifecycleNilFnIsSafe(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	s.OnLifecycle(nil) // must not panic
	_, tok, _ := s.Create("agent-hook-nil", "")
	sess, _ := s.Authenticate(tok)
	s.Revoke(sess.ID) // must not panic
}

// Revoke is idempotent while the session entry still exists.
func TestStore_RevokeTwiceReturnsTrue(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	_, tok, _ := s.Create("agent-rev2", "")
	sess, _ := s.Authenticate(tok)
	if !s.Revoke(sess.ID) {
		t.Fatal("first Revoke should succeed")
	}
	if !s.Revoke(sess.ID) {
		t.Error("second Revoke should still return true while the session exists")
	}
}
