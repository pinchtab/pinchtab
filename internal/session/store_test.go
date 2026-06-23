package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTouchDebouncesPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	base := time.Now()
	cur := base
	s := NewStore(Config{Enabled: true, IdleTimeout: time.Hour, MaxLifetime: 24 * time.Hour, PersistPath: path})
	s.now = func() time.Time { return cur }

	id, _, err := s.Create("agent", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	readLastSeen := func() time.Time {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read persist file: %v", err)
		}
		var ps persistedStore
		if err := json.Unmarshal(data, &ps); err != nil {
			t.Fatalf("unmarshal persist file: %v", err)
		}
		for _, rec := range ps.Sessions {
			if rec.ID == id {
				return rec.LastSeenAt
			}
		}
		t.Fatalf("session %q not in persist file", id)
		return time.Time{}
	}

	// First touch persists immediately (lastTouchSave is zero).
	cur = base.Add(time.Second)
	s.Touch(id)
	seen1 := readLastSeen()
	if !seen1.Equal(cur) {
		t.Fatalf("first touch: persisted LastSeen=%v, want %v", seen1, cur)
	}

	// Second touch within the debounce window must NOT rewrite the file.
	cur = base.Add(2 * time.Second)
	s.Touch(id)
	if seen2 := readLastSeen(); !seen2.Equal(seen1) {
		t.Fatalf("touch within window persisted (LastSeen %v -> %v); expected debounce", seen1, seen2)
	}

	// After the interval elapses, a touch persists again.
	cur = base.Add(2*time.Second + touchPersistInterval + time.Second)
	s.Touch(id)
	if seen3 := readLastSeen(); !seen3.Equal(cur) {
		t.Fatalf("touch after interval: persisted LastSeen=%v, want %v", seen3, cur)
	}
}

func TestAuthenticateTokenIndex_ManySessions(t *testing.T) {
	s := NewStore(Config{Enabled: true, IdleTimeout: 30 * time.Minute, MaxLifetime: 24 * time.Hour})

	type rec struct{ id, token string }
	recs := make([]rec, 0, 5)
	for i := 0; i < 5; i++ {
		id, token, err := s.Create("agent", "", "")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		recs = append(recs, rec{id, token})
	}

	// Each token must resolve (O(1) index) to its own session, not another.
	for i, r := range recs {
		sess, ok := s.Authenticate(r.token)
		if !ok || sess == nil {
			t.Fatalf("auth %d failed for valid token", i)
		}
		if sess.ID != r.id {
			t.Fatalf("auth %d returned session %q, want %q", i, sess.ID, r.id)
		}
	}

	// Unknown token fails.
	if _, ok := s.Authenticate("ses_nope"); ok {
		t.Fatal("authenticated an unknown token")
	}

	// Revoked session no longer authenticates (status filtered).
	if !s.Revoke(recs[0].id) {
		t.Fatal("revoke failed")
	}
	if _, ok := s.Authenticate(recs[0].token); ok {
		t.Fatal("authenticated a revoked session")
	}
	// Other sessions still authenticate.
	if _, ok := s.Authenticate(recs[1].token); !ok {
		t.Fatal("a non-revoked session stopped authenticating")
	}
}

func TestCreateAndAuthenticate(t *testing.T) {
	s := NewStore(Config{Enabled: true, IdleTimeout: 30 * time.Minute, MaxLifetime: 24 * time.Hour})
	id, token, err := s.Create("agent-1", "test session", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "ses_") {
		t.Fatalf("session ID should start with ses_, got %q", id)
	}
	if !strings.HasPrefix(token, "ses_") {
		t.Fatalf("token should start with ses_, got %q", token)
	}
	if len(token) != 4+48 { // ses_ + 48 hex chars
		t.Fatalf("token length should be 52, got %d", len(token))
	}
	if len(id) != 4+16 { // ses_ + 16 hex chars
		t.Fatalf("session ID length should be 20, got %d", len(id))
	}

	sess, ok := s.Authenticate(token)
	if !ok || sess == nil {
		t.Fatal("expected successful authentication")
	}
	if sess.AgentID != "agent-1" {
		t.Fatalf("expected agentId agent-1, got %q", sess.AgentID)
	}
	if sess.Label != "test session" {
		t.Fatalf("expected label 'test session', got %q", sess.Label)
	}
}

func TestAuthenticateInvalidToken(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	_, _, _ = s.Create("agent-1", "", "")
	sess, ok := s.Authenticate("ses_invalidtoken")
	if ok || sess != nil {
		t.Fatal("expected failed authentication for invalid token")
	}
}

func TestAuthenticateEmptyToken(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	sess, ok := s.Authenticate("")
	if ok || sess != nil {
		t.Fatal("expected failed authentication for empty token")
	}
}

func TestExpiry(t *testing.T) {
	s := NewStore(Config{Enabled: true, MaxLifetime: 1 * time.Hour, IdleTimeout: 30 * time.Minute})
	now := time.Now()
	s.now = func() time.Time { return now }

	_, token, _ := s.Create("agent-1", "", "")

	// Advance past max lifetime
	s.now = func() time.Time { return now.Add(2 * time.Hour) }
	sess, ok := s.Authenticate(token)
	if ok || sess != nil {
		t.Fatal("expected authentication to fail after expiry")
	}
}

func TestIdleTimeout(t *testing.T) {
	s := NewStore(Config{Enabled: true, MaxLifetime: 24 * time.Hour, IdleTimeout: 10 * time.Minute})
	now := time.Now()
	s.now = func() time.Time { return now }

	_, token, _ := s.Create("agent-1", "", "")

	// Advance past idle timeout
	s.now = func() time.Time { return now.Add(15 * time.Minute) }
	sess, ok := s.Authenticate(token)
	if ok || sess != nil {
		t.Fatal("expected authentication to fail after idle timeout")
	}
}

func TestIdleTimeoutReset(t *testing.T) {
	s := NewStore(Config{Enabled: true, MaxLifetime: 24 * time.Hour, IdleTimeout: 10 * time.Minute})
	now := time.Now()
	s.now = func() time.Time { return now }

	_, token, _ := s.Create("agent-1", "", "")

	// Advance 8 minutes and authenticate (resets idle)
	s.now = func() time.Time { return now.Add(8 * time.Minute) }
	sess, ok := s.Authenticate(token)
	if !ok || sess == nil {
		t.Fatal("expected auth to succeed within idle window")
	}

	// Another 8 minutes from the last auth (total 16 from create, but only 8 from last use)
	s.now = func() time.Time { return now.Add(16 * time.Minute) }
	sess, ok = s.Authenticate(token)
	if !ok || sess == nil {
		t.Fatal("expected auth to succeed after idle reset")
	}
}

func TestAuthenticateWithoutTouchDoesNotResetIdleTimeout(t *testing.T) {
	s := NewStore(Config{Enabled: true, MaxLifetime: 24 * time.Hour, IdleTimeout: 10 * time.Minute})
	now := time.Now()
	s.now = func() time.Time { return now }

	id, token, _ := s.Create("agent-1", "", "")
	sess, ok := s.Get(id)
	if !ok || sess == nil {
		t.Fatal("expected session to exist")
	}
	initialLastSeen := sess.LastSeenAt

	s.now = func() time.Time { return now.Add(8 * time.Minute) }
	sess, ok = s.AuthenticateWithoutTouch(token)
	if !ok || sess == nil {
		t.Fatal("expected auth without touch to succeed within idle window")
	}
	if !sess.LastSeenAt.Equal(initialLastSeen) {
		t.Fatal("AuthenticateWithoutTouch should not update LastSeenAt")
	}

	s.now = func() time.Time { return now.Add(11 * time.Minute) }
	sess, ok = s.Authenticate(token)
	if ok || sess != nil {
		t.Fatal("expected session to expire when it was never touched")
	}
}

func TestTouchResetsIdleTimeout(t *testing.T) {
	s := NewStore(Config{Enabled: true, MaxLifetime: 24 * time.Hour, IdleTimeout: 10 * time.Minute})
	now := time.Now()
	s.now = func() time.Time { return now }

	id, token, _ := s.Create("agent-1", "", "")

	s.now = func() time.Time { return now.Add(8 * time.Minute) }
	sess, ok := s.AuthenticateWithoutTouch(token)
	if !ok || sess == nil {
		t.Fatal("expected auth without touch to succeed")
	}
	if !s.Touch(id) {
		t.Fatal("expected touch to succeed")
	}

	s.now = func() time.Time { return now.Add(16 * time.Minute) }
	sess, ok = s.Authenticate(token)
	if !ok || sess == nil {
		t.Fatal("expected auth to succeed after touch reset")
	}
}

func TestRevoke(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	id, token, _ := s.Create("agent-1", "", "")

	ok := s.Revoke(id)
	if !ok {
		t.Fatal("expected revoke to succeed")
	}

	sess, ok := s.Authenticate(token)
	if ok || sess != nil {
		t.Fatal("expected authentication to fail after revoke")
	}
}

func TestRevokeNotFound(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	if s.Revoke("ses_nonexistent") {
		t.Fatal("expected revoke to return false for non-existent session")
	}
}

func TestGet(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	id, _, _ := s.Create("agent-1", "my label", "")

	sess, ok := s.Get(id)
	if !ok || sess == nil {
		t.Fatal("expected Get to find session")
	}
	if sess.AgentID != "agent-1" || sess.Label != "my label" {
		t.Fatal("unexpected session data")
	}
}

func TestList(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	_, _, _ = s.Create("agent-1", "", "")
	_, _, _ = s.Create("agent-2", "", "")

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
}

func TestPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := NewStore(Config{Enabled: true, PersistPath: path, IdleTimeout: 1 * time.Hour, MaxLifetime: 24 * time.Hour})
	_, token, _ := s1.Create("agent-1", "persist test", "")

	// Create a new store from the same file
	s2 := NewStore(Config{Enabled: true, PersistPath: path, IdleTimeout: 1 * time.Hour, MaxLifetime: 24 * time.Hour})

	sess, ok := s2.Authenticate(token)
	if !ok || sess == nil {
		t.Fatal("expected session to survive persistence round-trip")
	}
	if sess.AgentID != "agent-1" {
		t.Fatalf("expected agent-1, got %q", sess.AgentID)
	}
}

func TestPrunedOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := NewStore(Config{Enabled: true, PersistPath: path, MaxLifetime: 1 * time.Hour, IdleTimeout: 30 * time.Minute})
	now := time.Now()
	s1.now = func() time.Time { return now }
	_, _, _ = s1.Create("agent-1", "will expire", "")

	// Load with time advanced past expiry — set now before NewStore calls loadPersisted
	futureTime := now.Add(2 * time.Hour)
	s2 := &Store{
		sessions: make(map[string]*Session),
		now:      func() time.Time { return futureTime },
	}
	s2.applyConfig(Config{Enabled: true, PersistPath: path, MaxLifetime: 1 * time.Hour, IdleTimeout: 30 * time.Minute})
	s2.loadPersisted()

	if len(s2.List()) != 0 {
		t.Fatal("expected expired sessions to be pruned on load")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s := NewStore(Config{Enabled: true, PersistPath: path})
	_, _, _ = s.Create("agent-1", "", "")

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var store persistedStore
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("persisted file is not valid JSON: %v", err)
	}
	if len(store.Sessions) != 1 {
		t.Fatalf("expected 1 persisted session, got %d", len(store.Sessions))
	}
	// Verify no raw token in persisted data
	if strings.Contains(string(data), "ses_") && strings.Contains(string(data), `"token"`) {
		t.Fatal("raw token found in persisted data")
	}
}

func TestNilStore(t *testing.T) {
	var s *Store
	_, ok := s.Authenticate("token")
	if ok {
		t.Fatal("nil store should not authenticate")
	}
	if s.Revoke("id") {
		t.Fatal("nil store should not revoke")
	}
	if s.Enabled() {
		t.Fatal("nil store should not be enabled")
	}
	if s.List() != nil {
		t.Fatal("nil store should return nil list")
	}
}
