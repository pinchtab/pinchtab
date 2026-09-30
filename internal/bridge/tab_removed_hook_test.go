package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

// TestExternalTabRemovedHookSurvivesRewire verifies that a hook registered via
// Bridge.AddTabRemovedHook is applied to the current TabManager, re-applied when
// wireTabManager swaps the TabManager (launch/reinit/remote-CDP), and not
// duplicated across rewires — alongside the built-in dropFetchPauseSuppression
// and releaseTabLock.
func TestExternalTabRemovedHookSurvivesRewire(t *testing.T) {
	b := &Bridge{}

	var calls int
	b.AddTabRemovedHook(func(string) { calls++ })

	ctx := context.Background()
	b.wireTabManager(ctx)

	if got := len(b.onTabRemovedHooks); got != builtInTabRemovedHooks+1 {
		t.Fatalf("hooks after first wire = %d, want %d", got, builtInTabRemovedHooks+1)
	}
	for _, h := range b.onTabRemovedHooks {
		h("tab1")
	}
	if calls != 1 {
		t.Fatalf("external hook fired %d times, want 1", calls)
	}

	// A reinit swaps the TabManager; the external hook must persist without
	// duplicating (built-in is freshly re-added, not accumulated).
	b.wireTabManager(ctx)
	if got := len(b.onTabRemovedHooks); got != builtInTabRemovedHooks+1 {
		t.Fatalf("hooks after rewire = %d, want %d (no duplication)", got, builtInTabRemovedHooks+1)
	}
}

// builtInTabRemovedHooks is how many cleanups wireTabManager adds by itself.
// Naming it keeps the count above about duplication across a rewire, which is
// what that test is for, rather than about how many built-ins happen to exist.
const builtInTabRemovedHooks = 4

// The lease of a closed tab is dropped because releaseTabLock is WIRED, not
// merely because LockManager.Release exists. This drives the hook list the tab
// teardown runs, so a future wire-up that forgets it reds here rather than
// leaking quietly.
func TestClosingATabDropsItsLease(t *testing.T) {
	b := &Bridge{}
	b.Locks = NewLockManager()
	b.wireTabManager(context.Background())

	if err := b.Locks.TryLock("tab1", "agent-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := b.Locks.TryLock("tab2", "agent-b", time.Minute); err != nil {
		t.Fatal(err)
	}

	for _, h := range b.onTabRemovedHooks {
		h("tab1")
	}

	if b.Locks.Get("tab1") != nil {
		t.Error("the closed tab kept its lease; releaseTabLock is not reaching the teardown")
	}
	if b.Locks.Get("tab2") == nil {
		t.Error("closing one tab dropped another tab's lease")
	}
}

// wireTabManager runs before the constructor assigns Locks on the first wire, so
// the hook has to tolerate a nil manager rather than panic during startup.
func TestReleaseTabLockBeforeLocksExist(t *testing.T) {
	b := &Bridge{}
	b.wireTabManager(context.Background())
	for _, h := range b.onTabRemovedHooks {
		h("tab1")
	}
}

func TestClosingATabDropsItsPointerAndFingerprintState(t *testing.T) {
	b := New(context.Background(), nil, &config.RuntimeConfig{})
	b.wireTabManager(context.Background())
	b.rememberPointerPosition("tab1", 10, 20)
	b.rememberPointerPosition("tab2", 30, 40)
	b.SetFingerprintRotateActive("tab1", true)
	b.SetFingerprintRotateActive("tab2", true)

	for _, h := range b.onTabRemovedHooks {
		h("tab1")
	}

	if _, _, known := b.currentPointerPosition("tab1"); known {
		t.Error("closed tab kept its pointer position")
	}
	if b.FingerprintRotateActive("tab1") {
		t.Error("closed tab kept its fingerprint overlay")
	}
	if _, _, known := b.currentPointerPosition("tab2"); !known {
		t.Error("closing one tab dropped another tab's pointer position")
	}
	if !b.FingerprintRotateActive("tab2") {
		t.Error("closing one tab dropped another tab's fingerprint overlay")
	}

	b.wireTabManager(context.Background())
	b.rememberPointerPosition("tab3", 1, 1)
	for _, h := range b.onTabRemovedHooks {
		h("tab3")
	}
	if _, _, known := b.currentPointerPosition("tab3"); known {
		t.Error("pointer cleanup did not survive a TabManager rewire")
	}
}
