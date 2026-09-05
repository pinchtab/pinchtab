package browsersession

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func benchManager(b *testing.B, sessions int) (*Manager, []string) {
	b.Helper()
	mgr := NewManager(Config{
		IdleTimeout:     365 * 24 * time.Hour,
		MaxLifetime:     365 * 24 * time.Hour,
		ElevationWindow: 15 * time.Minute,
		Persist:         true,
		PersistPath:     filepath.Join(b.TempDir(), "sessions.json"),
	})
	ids := make([]string, 0, sessions)
	for i := 0; i < sessions; i++ {
		id, err := mgr.Create("secret")
		if err != nil {
			b.Fatalf("Create: %v", err)
		}
		ids = append(ids, id)
	}
	return mgr, ids
}

func BenchmarkElevateWithPersist(b *testing.B) {
	for _, sessions := range []int{1, 50, 500} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			mgr, ids := benchManager(b, sessions)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				mgr.Elevate(ids[i%len(ids)], "secret")
			}
		})
	}
}

// BenchmarkElevateStateLockWait reports how long an unrelated state-lock
// acquisition waits while Elevate persists — the cost this card removes.
func BenchmarkElevateStateLockWait(b *testing.B) {
	for _, sessions := range []int{1, 50, 500} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			mgr, ids := benchManager(b, sessions)
			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					default:
					}
					mgr.Elevate(ids[i%len(ids)], "secret")
				}
			}()

			var waited time.Duration
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				time.Sleep(100 * time.Microsecond)
				start := time.Now()
				mgr.mu.Lock()
				sessions := len(mgr.sessions)
				mgr.mu.Unlock()
				waited += time.Since(start)
				if sessions == 0 {
					b.Fatal("sessions drained")
				}
			}
			b.StopTimer()
			close(stop)
			<-done
			b.ReportMetric(float64(waited.Nanoseconds())/float64(b.N), "lockwait-ns/op")
		})
	}
}

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

func TestPersistPathReportsConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	mgr := NewManager(Config{Persist: true, PersistPath: path})
	if got := mgr.PersistPath(); got != path {
		t.Fatalf("PersistPath() = %q, want %q", got, path)
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

// No filesystem call may happen while the state lock is held — the property the two-lock
// split exists to establish. Every writeSnapshot call site in session.go is driven here:
// Create, withValidSession (through Elevate), Revoke and UpdateConfig.
//
// loadPersisted is the fifth site and is deliberately NOT driven, inspected rather than
// pinned. The reason is a CONDITION, not a category: it has exactly one caller, inside
// NewManager, which runs it after unlocking and before returning the manager — so no other
// goroutine can hold a reference yet and a held lock has nothing to interact with. A second
// caller of loadPersisted, or a call after the manager is published, retires that reasoning
// and this site needs driving like the rest. Installing beforeWrite is impossible before
// NewManager returns, so covering it would mean a production seam on a startup-only path.
func TestPersistWritesWithStateLockReleased(t *testing.T) {
	// Exact, not a floor. writeSnapshot returns before it ever calls beforeWrite when the
	// job carries no path, and snapshotLocked yields an empty job whenever the manager
	// stops persisting — so a site that quietly stopped writing would leave this guard
	// driving fewer sites than it names while a non-zero check still passed. That is the
	// same silent vacuity the guard exists to catch, so the count is the assertion.
	const wantWrites = 4

	path := filepath.Join(t.TempDir(), "sessions.json")
	cfg := Config{
		IdleTimeout:     time.Hour,
		MaxLifetime:     24 * time.Hour,
		ElevationWindow: time.Minute,
		Persist:         true,
		PersistPath:     path,
	}
	mgr := NewManager(cfg)

	writes := 0
	available := 0
	mgr.beforeWrite = func() {
		writes++
		if mgr.mu.TryLock() {
			available++
			mgr.mu.Unlock()
		}
	}

	sessionID, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !mgr.Elevate(sessionID, "secret") {
		t.Fatal("Elevate() = false, want true")
	}
	mgr.Revoke(sessionID)
	// The same config deliberately: Persist must stay true or this call contributes no
	// observed write at all, and the path must stay the same or UpdateConfig also removes
	// the old file, adding a filesystem side effect to a test about lock ordering.
	mgr.UpdateConfig(cfg)

	if writes != wantWrites {
		t.Fatalf("observed %d persist writes, want %d — one per writeSnapshot site this test drives; a site that stopped writing is not covered by the lock assertion below", writes, wantWrites)
	}
	if available != writes {
		t.Errorf("state lock was held during %d of %d writes, want 0", writes-available, writes)
	}
}

func TestWriteSnapshotDiscardsSupersededWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	mgr := NewManager(Config{
		IdleTimeout: time.Hour,
		MaxLifetime: 24 * time.Hour,
		Persist:     true,
		PersistPath: path,
	})

	first, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mgr.mu.Lock()
	older := mgr.snapshotLocked()
	second, err := randomSessionID()
	if err != nil {
		t.Fatalf("randomSessionID: %v", err)
	}
	mgr.sessions[second] = mgr.sessions[first]
	newer := mgr.snapshotLocked()
	mgr.mu.Unlock()

	// The newer snapshot lands first; the in-flight older one must not regress it.
	mgr.writeSnapshot(newer)
	mgr.writeSnapshot(older)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persist file: %v", err)
	}
	var ps persistedSessions
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ps.Sessions) != 2 {
		t.Fatalf("persist file has %d sessions, want 2 — the older snapshot overwrote the newer", len(ps.Sessions))
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

// A dashboard login that is never presented again — a closed browser, a second
// device — expires but is noticed by nothing: every other deletion path runs off
// the cookie the abandoned session no longer sends. Without a sweep its record and
// its token hash sit in memory and in the persisted file for the life of the
// process, and the file is rewritten whole on every later mutation.
func TestPruneExpiredForgetsAnAbandonedSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	mgr := NewManager(Config{
		IdleTimeout: time.Hour,
		MaxLifetime: 24 * time.Hour,
		Persist:     true,
		PersistPath: path,
	})
	mgr.now = func() time.Time { return now }

	abandoned, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	now = now.Add(90 * time.Minute)
	live, err := mgr.Create("secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mgr.PruneExpired()

	persisted := func() map[string]bool {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read persist file: %v", readErr)
		}
		var ps persistedSessions
		if unmarshalErr := json.Unmarshal(data, &ps); unmarshalErr != nil {
			t.Fatalf("unmarshal: %v", unmarshalErr)
		}
		ids := map[string]bool{}
		for _, r := range ps.Sessions {
			ids[r.ID] = true
		}
		return ids
	}()

	mgr.mu.RLock()
	_, stillInMemory := mgr.sessions[abandoned]
	_, liveInMemory := mgr.sessions[live]
	mgr.mu.RUnlock()

	if stillInMemory {
		t.Error("the expired session is still in memory after the sweep")
	}
	if persisted[abandoned] {
		t.Error("the expired session's record and token hash are still on disk after the sweep")
	}
	if !liveInMemory || !persisted[live] {
		t.Error("the sweep dropped a session that is still within its idle timeout")
	}
}
