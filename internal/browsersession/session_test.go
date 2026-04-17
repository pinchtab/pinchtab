package browsersession

import (
	"path/filepath"
	"testing"
	"time"
)

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
