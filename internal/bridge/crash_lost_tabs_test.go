package bridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func bridgeHoldingTabs(t *testing.T, ids ...string) *Bridge {
	t.Helper()
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b := newTestBridge()
	b.BrowserCtx = ctx
	for _, id := range ids {
		b.tabs[id] = &TabEntry{CreatedAt: time.Now()}
	}
	return b
}

// After the browser dies the relaunch starts a fresh tab manager, so every tab id
// the caller holds misses. The miss must say the crash removed the tab, not that
// the id was wrong, and it must keep saying so after the relaunch (a later
// generation), because the tab belonged to the browser that died.
func TestATabLostToABrowserDeathIsReportedAsLostWhateverBrowserAnswers(t *testing.T) {
	b := bridgeHoldingTabs(t, "tabA", "tabB")
	event := b.recordBrowserDeath("unexpected context cancellation")

	relaunched := NewTabManager(context.Background(), nil, nil, nil, nil)
	_, _, err := relaunched.TabContext("tabA")
	var lost *TabNotFoundError
	if !errors.As(err, &lost) {
		t.Fatalf("TabContext = %v, want a TabNotFoundError", err)
	}
	if lost.TabID != "tabA" || err.Error() != "tab tabA not found" {
		t.Errorf("error = %q, want the tab named as not found", err)
	}
	if lost.Crash == nil || lost.Crash.Reason != event.Reason {
		t.Fatalf("crash = %+v, want the browser death %+v", lost.Crash, event)
	}
	if got, ok := CrashThatDestroyedTab("tabB"); !ok || got.Reason != event.Reason {
		t.Errorf("tabB: %+v %v, want the same death", got, ok)
	}
}

func TestAnUnknownTabIsNotBlamedOnACrash(t *testing.T) {
	b := bridgeHoldingTabs(t, "tabA")
	b.recordBrowserDeath("inspector.targetCrashed")

	_, _, err := NewTabManager(context.Background(), nil, nil, nil, nil).TabContext("never-existed")
	var lost *TabNotFoundError
	if !errors.As(err, &lost) {
		t.Fatalf("TabContext = %v, want a TabNotFoundError", err)
	}
	if lost.Crash != nil {
		t.Errorf("crash = %+v on a tab the browser never held", lost.Crash)
	}
}

func TestABrowserDeathCountsInTheCrashSummary(t *testing.T) {
	b := bridgeHoldingTabs(t, "tabA")
	b.recordBrowserDeath("unexpected context cancellation")
	summary := CrashSnapshot()
	if summary.Total != 1 || len(summary.Recent) != 1 || summary.Recent[0].Reason != "unexpected context cancellation" {
		t.Errorf("summary = %+v, want the one death", summary)
	}
	if _, ok := CrashForBrowserContext(b.BrowserCtx); !ok {
		t.Errorf("the death is not current for the browser context it happened on")
	}
}

func TestTheLostTabRecordIsBounded(t *testing.T) {
	ids := make([]string, maxTabsLostToCrashes+5)
	for i := range ids {
		ids[i] = "tab" + string(rune('A'+i%26)) + string(rune('a'+i/26))
	}
	b := bridgeHoldingTabs(t, ids...)
	b.recordBrowserDeath("x")
	kept := 0
	for _, id := range ids {
		if _, ok := CrashThatDestroyedTab(id); ok {
			kept++
		}
	}
	if kept != maxTabsLostToCrashes {
		t.Errorf("kept %d lost tabs, want the bound %d", kept, maxTabsLostToCrashes)
	}
}

func watchedBridgeHoldingTabs(t *testing.T, ids ...string) (*Bridge, context.CancelFunc, chan CrashEvent) {
	t.Helper()
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b := newTestBridge()
	b.BrowserCtx = ctx
	for _, id := range ids {
		b.tabs[id] = &TabEntry{CreatedAt: time.Now()}
	}
	deaths := make(chan CrashEvent, 1)
	go b.watchBrowserDeath(ctx, func(ev CrashEvent) { deaths <- ev })
	return b, cancel, deaths
}

// The production trigger, not the helper: the browser context ending outside a
// drain is what a kill -9 of Chrome reaches, and it must record the tabs the
// browser held so the next lookup of one names the death.
func TestTheBrowserContextEndingRecordsTheDeathAndTheTabsItTook(t *testing.T) {
	_, kill, deaths := watchedBridgeHoldingTabs(t, "tabA", "tabB")
	kill()

	var death CrashEvent
	select {
	case death = <-deaths:
	case <-time.After(2 * time.Second):
		t.Fatal("the browser context ended and no death was recorded")
	}
	if death.Reason != "unexpected context cancellation" {
		t.Errorf("reason = %q, want the cancellation named", death.Reason)
	}
	if summary := CrashSnapshot(); summary.Total != 1 {
		t.Errorf("crash total = %d, want the one death", summary.Total)
	}

	_, _, err := NewTabManager(context.Background(), nil, nil, nil, nil).TabContext("tabB")
	var lost *TabNotFoundError
	if !errors.As(err, &lost) || lost.Crash == nil || lost.Crash.Reason != death.Reason {
		t.Fatalf("TabContext after the death = %v, want the tab reported as lost to it", err)
	}
}

// A drain ends the context on purpose; it must record neither a crash nor lost tabs,
// or every clean shutdown would annotate the next session's misses.
func TestADrainEndingTheBrowserContextRecordsNothing(t *testing.T) {
	b, stop, deaths := watchedBridgeHoldingTabs(t, "tabA")
	b.draining = true
	stop()

	select {
	case ev := <-deaths:
		t.Fatalf("a drain was recorded as a death: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
	if summary := CrashSnapshot(); summary.Total != 0 {
		t.Errorf("crash total = %d after a drain, want 0", summary.Total)
	}
	if _, lost := CrashThatDestroyedTab("tabA"); lost {
		t.Error("a drained tab was recorded as lost to a crash")
	}
}
