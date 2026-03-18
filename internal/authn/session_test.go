package authn

import (
	"testing"
	"time"
)

func TestSessionManagerValidateAndExpiry(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	mgr := NewSessionManager(SessionConfig{
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
	mgr := NewSessionManager(SessionConfig{})
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
	mgr := NewSessionManager(SessionConfig{
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
