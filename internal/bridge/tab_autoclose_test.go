package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

// newAutoCloseTM constructs a TabManager wired with the given lifecycle policy
// and inserts a stub TabEntry under tabID.
func newAutoCloseTM(t *testing.T, policy string, delay time.Duration, tabID string) *TabManager {
	t.Helper()
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{
		TabLifecyclePolicy: policy,
		TabCloseDelay:      delay,
	}, nil, nil, nil)
	tm.tabs[tabID] = &TabEntry{
		CDPID:     tabID,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	return tm
}

func TestScheduleAutoClose_NoOpWhenLifecycleKeep(t *testing.T) {
	tm := newAutoCloseTM(t, "keep", time.Second, "tab1")

	tm.ScheduleAutoClose("tab1")

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tabs["tab1"].autoCloseTimer != nil {
		t.Fatal("timer should not be armed when lifecycle is 'keep'")
	}
	if tm.tabs["tab1"].autoCloseGen != 0 {
		t.Fatalf("gen should not advance on no-op; got %d", tm.tabs["tab1"].autoCloseGen)
	}
}

func TestScheduleAutoClose_ArmsWhenCloseAfterUse(t *testing.T) {
	// Use a long delay so the timer doesn't fire during the assertions.
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")

	tm.ScheduleAutoClose("tab1")

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry := tm.tabs["tab1"]
	if entry.autoCloseTimer == nil {
		t.Fatal("timer should be armed when lifecycle is close_idle")
	}
	if entry.autoCloseGen != 1 {
		t.Fatalf("gen should advance to 1 on first schedule; got %d", entry.autoCloseGen)
	}
}

func TestScheduleAutoClose_ResetBumpsGenerationAndStopsPriorTimer(t *testing.T) {
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")

	tm.ScheduleAutoClose("tab1")
	tm.mu.RLock()
	first := tm.tabs["tab1"].autoCloseTimer
	tm.mu.RUnlock()

	tm.ScheduleAutoClose("tab1")

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry := tm.tabs["tab1"]
	if entry.autoCloseGen != 2 {
		t.Fatalf("gen should advance to 2 on reset; got %d", entry.autoCloseGen)
	}
	if entry.autoCloseTimer == nil {
		t.Fatal("timer should still be armed after reset")
	}
	if entry.autoCloseTimer == first {
		t.Fatal("expected a fresh timer instance after reset")
	}
}

func TestCancelAutoClose_StopsTimerAndBumpsGeneration(t *testing.T) {
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")

	tm.ScheduleAutoClose("tab1")
	tm.CancelAutoClose("tab1")

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry := tm.tabs["tab1"]
	if entry.autoCloseTimer != nil {
		t.Fatal("timer should be nil after cancel")
	}
	if entry.autoCloseGen != 2 {
		t.Fatalf("gen should advance on cancel; got %d", entry.autoCloseGen)
	}
}

func TestScheduleAutoClose_UnknownTabIsNoOp(t *testing.T) {
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")

	// Should not panic or affect the existing tab.
	tm.ScheduleAutoClose("missing")

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tabs["tab1"].autoCloseTimer != nil {
		t.Fatal("unrelated tab must not be touched")
	}
}

func TestCancelAutoClose_UnknownTabIsNoOp(t *testing.T) {
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")
	// Should not panic.
	tm.CancelAutoClose("missing")
}

func TestAutoCloseFire_StaleGenerationIsNoOp(t *testing.T) {
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")

	tm.ScheduleAutoClose("tab1")
	tm.mu.RLock()
	staleGen := tm.tabs["tab1"].autoCloseGen
	tm.mu.RUnlock()

	// Cancel bumps gen, so the captured gen is now stale.
	tm.CancelAutoClose("tab1")

	// Re-arm to make sure the entry is still present and timer is set again.
	tm.ScheduleAutoClose("tab1")

	// Fire with the stale generation: must be a no-op (no CloseTab call,
	// timer untouched).
	tm.autoCloseFire("tab1", staleGen)

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry, ok := tm.tabs["tab1"]
	if !ok {
		t.Fatal("entry should not be removed by stale fire")
	}
	if entry.autoCloseTimer == nil {
		t.Fatal("active timer should not be cleared by stale fire")
	}
}

func TestPurgeTrackedTabState_StopsAutoCloseTimer(t *testing.T) {
	tm := newAutoCloseTM(t, "close_idle", time.Hour, "tab1")
	// purge needs a Cancel func so lookupTrackedTabForCleanup returns ok=true.
	tm.tabs["tab1"].Cancel = func() {}

	tm.ScheduleAutoClose("tab1")

	if !tm.purgeTrackedTabState("tab1", "tab1") {
		t.Fatal("purgeTrackedTabState should report true for a tracked tab")
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if _, ok := tm.tabs["tab1"]; ok {
		t.Fatal("entry should have been removed by purge")
	}
}
