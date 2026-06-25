package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestStore_ConcurrentMutationsPersistConsistently drives many parallel
// Create/Authenticate/Touch/Revoke calls (run with -race) to confirm narrowing
// the persistence lock keeps writes ordered and the on-disk file consistent: it
// must always unmarshal cleanly, and every persisted session must be one the
// store still knows about (no torn/stale snapshot, no phantom sessions).
func TestStore_ConcurrentMutationsPersistConsistently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := NewStore(Config{
		Enabled:     true,
		IdleTimeout: time.Hour,
		MaxLifetime: 24 * time.Hour,
		PersistPath: path,
	})

	const goroutines = 16
	const perGoroutine = 25

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				id, token, err := s.Create("agent", "", "")
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				if sess, ok := s.Authenticate(token); !ok || sess.ID != id {
					t.Errorf("Authenticate failed for freshly created session %s", id)
					return
				}
				s.Touch(id)
				if i%3 == 0 {
					s.Revoke(id)
				}
			}
		}(g)
	}
	wg.Wait()

	// The persist file must always be parseable (never torn by an interleaved write).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persist file: %v", err)
	}
	var ps persistedStore
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("persist file is not consistent JSON: %v\n%s", err, data)
	}

	// Every persisted session must still be known to the store (active ones are
	// kept in memory; the snapshot is a value-copy of that map), so the on-disk
	// set is coherent — no phantom/stale entries from an out-of-order write.
	known := make(map[string]struct{})
	for _, sess := range s.List() {
		known[sess.ID] = struct{}{}
	}
	for _, rec := range ps.Sessions {
		if _, ok := known[rec.ID]; !ok {
			t.Errorf("persisted session %s is not known to the store (stale/torn snapshot)", rec.ID)
		}
	}
}

// TestStore_WriteSnapshotSkipsStale asserts the sequence guard: a snapshot with a
// seq not greater than one already written is dropped, so a stale snapshot can
// never clobber a fresher one even if writers run out of order.
func TestStore_WriteSnapshotSkipsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := NewStore(Config{Enabled: true, PersistPath: path, IdleTimeout: time.Hour, MaxLifetime: 24 * time.Hour})

	fresh := snapshotJob{snapshot: persistedStore{SavedAt: time.Unix(200, 0).UTC()}, seq: 5}
	stale := snapshotJob{snapshot: persistedStore{SavedAt: time.Unix(100, 0).UTC()}, seq: 3}

	s.writeSnapshot(fresh)
	s.writeSnapshot(stale) // lower seq → must be skipped

	if s.writtenSeq != 5 {
		t.Fatalf("writtenSeq = %d, want 5 (stale write must not advance it)", s.writtenSeq)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ps persistedStore
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !ps.SavedAt.Equal(time.Unix(200, 0).UTC()) {
		t.Fatalf("on-disk SavedAt = %v, want the fresh snapshot (stale must not clobber)", ps.SavedAt)
	}
}
