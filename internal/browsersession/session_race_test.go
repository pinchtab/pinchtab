package browsersession

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The duration accessors are read on the per-request auth path while the
// dashboard config API can call UpdateConfig concurrently.
func TestDurationAccessorsAreSynchronized(t *testing.T) {
	m := NewManager(Config{IdleTimeout: time.Hour, MaxLifetime: time.Hour, ElevationWindow: time.Minute})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 500 {
			m.UpdateConfig(Config{
				IdleTimeout:     time.Duration(i+1) * time.Minute,
				MaxLifetime:     time.Duration(i+1) * time.Hour,
				ElevationWindow: time.Duration(i+1) * time.Second,
			})
		}
	}()
	go func() {
		defer wg.Done()
		for range 500 {
			_ = m.MaxLifetime()
			_ = m.IdleTimeout()
			_ = m.ElevationWindow()
		}
	}()
	wg.Wait()
}

// The writer runs outside the state lock, so every config field it needs must
// be captured in the snapshot rather than read while UpdateConfig is in flight.
func TestPersistConfigCapturedUnderStateLock(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(Config{
		IdleTimeout:     time.Hour,
		MaxLifetime:     time.Hour,
		ElevationWindow: time.Minute,
		Persist:         true,
		PersistPath:     filepath.Join(dir, "sessions-a.json"),
	})

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 200 {
			mgr.UpdateConfig(Config{
				IdleTimeout:                   time.Hour,
				MaxLifetime:                   time.Hour,
				ElevationWindow:               time.Duration(i+1) * time.Second,
				Persist:                       true,
				PersistPath:                   filepath.Join(dir, fmt.Sprintf("sessions-%d.json", i%3)),
				PersistElevationAcrossRestart: i%2 == 0,
			})
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			mgr.Elevate(sessionID, "secret")
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			if _, err := mgr.Create("secret"); err != nil {
				t.Errorf("Create: %v", err)
				return
			}
		}
	}()
	wg.Wait()
}

// The persist file is written via a temp file + rename; the temp must never
// survive a successful save.
func TestPersistLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard-auth-sessions.json")

	mgr := NewManager(Config{
		IdleTimeout: time.Hour,
		MaxLifetime: time.Hour,
		Persist:     true,
		PersistPath: path,
	})
	if _, err := mgr.Create("secret"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file %q left behind after save", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("persist dir has %d entries, want 1", len(entries))
	}
}
