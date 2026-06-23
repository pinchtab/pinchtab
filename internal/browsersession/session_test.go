package browsersession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsElevatedPersistsExpiryDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	base := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	cur := base
	mgr := NewManager(Config{
		IdleTimeout: time.Hour,
		MaxLifetime: 24 * time.Hour,
		Persist:     true,
		PersistPath: path,
	})
	mgr.now = func() time.Time { return cur }

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	hasSession := func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read persist file: %v", err)
		}
		var ps persistedSessions
		if err := json.Unmarshal(data, &ps); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, r := range ps.Sessions {
			if r.ID == sessionID {
				return true
			}
		}
		return false
	}

	if !hasSession() {
		t.Fatal("session not persisted after Create")
	}

	// Advance past the idle timeout so the session is expired/invalid, then check
	// elevation: IsElevated must delete AND persist the deletion.
	cur = base.Add(2 * time.Hour)
	if mgr.IsElevated(sessionID, "secret") {
		t.Fatal("IsElevated on an expired session = true, want false")
	}
	if hasSession() {
		t.Fatal("expired session still on disk — IsElevated did not persist its deletion")
	}
}

func TestValidateDebouncesLastSeenPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	base := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	cur := base
	mgr := NewManager(Config{
		IdleTimeout: time.Hour,
		MaxLifetime: 24 * time.Hour,
		Persist:     true,
		PersistPath: path,
	})
	mgr.now = func() time.Time { return cur }

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	readLastSeen := func() time.Time {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read persist file: %v", err)
		}
		var ps persistedSessions
		if err := json.Unmarshal(data, &ps); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, r := range ps.Sessions {
			if r.ID == sessionID {
				return r.LastSeen
			}
		}
		t.Fatalf("session %q not in persist file", sessionID)
		return time.Time{}
	}

	// First Validate persists immediately (lastTouchSave is zero).
	cur = base.Add(time.Second)
	mgr.Validate(sessionID, "secret")
	seen1 := readLastSeen()
	if !seen1.Equal(cur) {
		t.Fatalf("first validate: persisted LastSeen=%v, want %v", seen1, cur)
	}

	// Second Validate within the debounce window must NOT rewrite the file.
	cur = base.Add(2 * time.Second)
	mgr.Validate(sessionID, "secret")
	if seen2 := readLastSeen(); !seen2.Equal(seen1) {
		t.Fatalf("validate within window persisted (LastSeen %v -> %v); expected debounce", seen1, seen2)
	}

	// After the interval elapses, a Validate persists again.
	cur = base.Add(2*time.Second + touchPersistInterval + time.Second)
	mgr.Validate(sessionID, "secret")
	if seen3 := readLastSeen(); !seen3.Equal(cur) {
		t.Fatalf("validate after interval: persisted LastSeen=%v, want %v", seen3, cur)
	}
}

func TestSessionManagerValidateAndExpiry(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	mgr := NewManager(Config{
		IdleTimeout: time.Hour,
		MaxLifetime: 24 * time.Hour,
	})
	mgr.now = func() time.Time { return now }

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !mgr.Validate(sessionID, "secret") {
		t.Fatal("Validate() = false, want true")
	}

	now = now.Add(30 * time.Minute)
	if !mgr.Validate(sessionID, "secret") {
		t.Fatal("Validate() after activity = false, want true")
	}

	now = now.Add(61 * time.Minute)
	if mgr.Validate(sessionID, "secret") {
		t.Fatal("Validate() after idle expiry = true, want false")
	}
}

func TestSessionManagerInvalidatesOnTokenChange(t *testing.T) {
	mgr := NewManager(Config{})
	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if mgr.Validate(sessionID, "rotated-token") {
		t.Fatal("Validate() with rotated token = true, want false")
	}
}

func TestSessionManagerElevationWindow(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	mgr := NewManager(Config{
		IdleTimeout:     time.Hour,
		MaxLifetime:     24 * time.Hour,
		ElevationWindow: 15 * time.Minute,
	})
	mgr.now = func() time.Time { return now }

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if mgr.IsElevated(sessionID, "secret") {
		t.Fatal("IsElevated() before elevation = true, want false")
	}
	if !mgr.Elevate(sessionID, "secret") {
		t.Fatal("Elevate() = false, want true")
	}
	if !mgr.IsElevated(sessionID, "secret") {
		t.Fatal("IsElevated() after elevation = false, want true")
	}

	now = now.Add(16 * time.Minute)
	if mgr.IsElevated(sessionID, "secret") {
		t.Fatal("IsElevated() after elevation expiry = true, want false")
	}
}

func TestSessionManagerPersistsAcrossRestart(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "dashboard-auth-sessions.json")

	mgr := NewManager(Config{
		IdleTimeout: 365 * 24 * time.Hour,
		MaxLifetime: 365 * 24 * time.Hour,
		Persist:     true,
		PersistPath: path,
	})
	mgr.now = func() time.Time { return now }

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !mgr.Validate(sessionID, "secret") {
		t.Fatal("Validate() before restart = false, want true")
	}

	restarted := NewManager(Config{
		IdleTimeout: 365 * 24 * time.Hour,
		MaxLifetime: 365 * 24 * time.Hour,
		Persist:     true,
		PersistPath: path,
	})
	restarted.now = func() time.Time { return now }

	if !restarted.Validate(sessionID, "secret") {
		t.Fatal("Validate() after restart = false, want true")
	}
}

func TestSessionManagerClearsElevationAcrossRestartByDefault(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "dashboard-auth-sessions.json")

	mgr := NewManager(Config{
		IdleTimeout:     365 * 24 * time.Hour,
		MaxLifetime:     365 * 24 * time.Hour,
		ElevationWindow: 15 * time.Minute,
		Persist:         true,
		PersistPath:     path,
	})
	mgr.now = func() time.Time { return now }

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !mgr.Elevate(sessionID, "secret") {
		t.Fatal("Elevate() = false, want true")
	}

	restarted := NewManager(Config{
		IdleTimeout:     365 * 24 * time.Hour,
		MaxLifetime:     365 * 24 * time.Hour,
		ElevationWindow: 15 * time.Minute,
		Persist:         true,
		PersistPath:     path,
	})
	restarted.now = func() time.Time { return now }

	if restarted.IsElevated(sessionID, "secret") {
		t.Fatal("IsElevated() after restart = true, want false when persistence across restart is disabled")
	}
}
