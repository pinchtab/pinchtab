package tabs

import (
	"fmt"
	"testing"
	"time"
)

// A lease is a promise about a tab. When the tab is gone the promise has nobody
// left to keep, and the entry holding it is only memory.
//
// Nothing dropped it: purgeTrackedTabState clears the TabManager's own per-tab
// maps and notifies the dialog, executor, log and route managers, but the lock
// manager hangs off the Bridge and was not among them. Measured before Release
// existed: 1000 tabs locked and closed left 1000 entries.
func TestReleaseDropsTheLeaseOfATabThatIsGone(t *testing.T) {
	m := NewLockManager()
	const tabs = 1000

	for i := range tabs {
		if err := m.TryLock(fmt.Sprintf("tab-%d", i), "agent-a", time.Minute); err != nil {
			t.Fatalf("lock tab-%d: %v", i, err)
		}
	}
	if got := m.trackedForTest(); got != tabs {
		t.Fatalf("held %d leases after locking %d tabs", got, tabs)
	}

	for i := range tabs {
		m.Release(fmt.Sprintf("tab-%d", i))
	}
	if got := m.trackedForTest(); got != 0 {
		t.Errorf("%d leases survive tabs that no longer exist", got)
	}
}

// Release answers to the tab, not to the owner. Requiring the owner would leave
// an entry no call could remove whenever an agent closed a tab it had locked
// without unlocking it first — which is the case that produced the leak.
func TestReleaseDoesNotRequireTheOwner(t *testing.T) {
	m := NewLockManager()
	if err := m.TryLock("tab1", "agent-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := m.Unlock("tab1", "agent-b"); err == nil {
		t.Fatal("Unlock accepted the wrong owner; that refusal is what makes Release necessary")
	}

	m.Release("tab1")
	if m.Get("tab1") != nil {
		t.Error("the lease survived Release")
	}
	if got := m.trackedForTest(); got != 0 {
		t.Errorf("%d entries retained after Release", got)
	}
}

// Releasing a tab that holds no lease is what a close after an unlock looks
// like, so it has to be a no-op rather than a panic or a resurrected entry.
func TestReleaseOfAnUnlockedTabIsHarmless(t *testing.T) {
	m := NewLockManager()
	m.Release("never-locked")
	if got := m.trackedForTest(); got != 0 {
		t.Errorf("Release created %d entries for a tab that held no lease", got)
	}

	if err := m.TryLock("tab1", "agent-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := m.Unlock("tab1", "agent-a"); err != nil {
		t.Fatal(err)
	}
	m.Release("tab1")
	if got := m.trackedForTest(); got != 0 {
		t.Errorf("%d entries after unlock followed by release", got)
	}
}

// The lease a live tab still holds must be unaffected: Release names one tab.
func TestReleaseTouchesOnlyTheTabItNames(t *testing.T) {
	m := NewLockManager()
	if err := m.TryLock("closing", "agent-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := m.TryLock("still-open", "agent-b", time.Minute); err != nil {
		t.Fatal(err)
	}

	m.Release("closing")

	if m.Get("closing") != nil {
		t.Error("the closed tab kept its lease")
	}
	live := m.Get("still-open")
	if live == nil {
		t.Fatal("releasing one tab dropped another tab's lease")
	}
	if live.Owner != "agent-b" {
		t.Errorf("surviving lease owner = %q, want agent-b", live.Owner)
	}
}
