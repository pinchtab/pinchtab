package session

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// cfg is written by UpdateConfig under s.mu, so every reader must hold s.mu too.
// One did not: Create built the session's expiry from s.cfg.MaxLifetime before it
// took the lock, which is a data race by the memory model and reads a value a
// concurrent save may be part-way through replacing.
//
// The rule cannot be checked by a unit test — an unlocked read returns the right
// answer almost always, and the race detector only reports it under an
// interleaving no test can force — so the guard is a source census over the
// functions allowed to touch the field.
func TestEveryConfigReadHoldsTheStoreLock(t *testing.T) {
	pkg := srccensus.Load(t, ".", 4)

	// Each entry is a function that touches s.cfg while holding s.mu, or that runs
	// before the store is shared. Checked in both directions: a function that stops
	// touching cfg fails too, so the list cannot drift from the real one.
	owners := []string{
		"applyConfig",     // called under the lock by UpdateConfig, and from NewStore before sharing
		"registerSession", // takes the lock, stamps the lifetime, installs the session
		"Enabled",         // takes the lock for the read
		"Mode",            // takes the lock for the read
		"PersistPath",     // takes the lock for the read
		"isExpired",       // only ever called with the lock held
		"loadPersisted",   // runs in NewStore, before the store is reachable
		"snapshotLocked",
	}

	spans := make(map[string]srccensus.Func, len(owners))
	for _, name := range owners {
		fn, ok := pkg.Func(name)
		if !ok {
			t.Fatalf("owner %s of s.cfg no longer exists in %s; re-point this census at whatever replaced it rather than deleting it", name, pkg.Dir())
		}
		spans[name] = fn
	}

	sites := pkg.FieldReferences("cfg")
	if len(sites) < len(owners) {
		t.Fatalf("only %d reference(s) to s.cfg for %d owner(s); the census is matching almost nothing and would pass vacuously", len(sites), len(owners))
	}

	covered := map[string]bool{}
	for _, site := range sites {
		owner := ""
		for name, span := range spans {
			if pkg.Contains(span, site) {
				owner = name
				break
			}
		}
		covered[owner] = true
		if owner == "" {
			t.Errorf("%s touches s.cfg outside a function that holds s.mu; take the lock there, or add it here with the reason it is safe", site)
		}
	}
	for _, name := range owners {
		if !covered[name] {
			t.Errorf("%s no longer touches s.cfg, so this census is guarding one site fewer than it claims; drop it from the list", name)
		}
	}
}
