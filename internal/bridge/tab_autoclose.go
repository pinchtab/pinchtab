package bridge

import (
	"log/slog"
	"time"
)

// ScheduleAutoClose (re)arms the per-tab idle close timer when the lifecycle
// policy is "close_idle". Idempotent: any prior timer for the tab is
// stopped first. Safe to call from handler goroutines.
func (tm *TabManager) ScheduleAutoClose(tabID string) {
	if tm == nil || tm.config == nil {
		return
	}
	if tm.config.TabLifecyclePolicy != "close_idle" {
		return
	}
	delay := tm.config.TabCloseDelay
	if delay <= 0 {
		return
	}

	tm.mu.Lock()
	entry, ok := tm.tabs[tabID]
	if !ok {
		tm.mu.Unlock()
		return
	}
	if entry.autoCloseTimer != nil {
		entry.autoCloseTimer.Stop()
	}
	entry.autoCloseGen++
	gen := entry.autoCloseGen
	entry.autoCloseTimer = time.AfterFunc(delay, func() {
		tm.autoCloseFire(tabID, gen)
	})
	tm.mu.Unlock()
}

// CancelAutoClose stops the per-tab idle close timer if any. Safe to call when
// no timer is armed; bumps the generation so an already-fired-but-not-yet-run
// callback will recognise itself as stale.
func (tm *TabManager) CancelAutoClose(tabID string) {
	if tm == nil {
		return
	}
	tm.mu.Lock()
	entry, ok := tm.tabs[tabID]
	if !ok {
		tm.mu.Unlock()
		return
	}
	if entry.autoCloseTimer != nil {
		entry.autoCloseTimer.Stop()
		entry.autoCloseTimer = nil
	}
	entry.autoCloseGen++
	tm.mu.Unlock()
}

func (tm *TabManager) autoCloseFire(tabID string, gen uint64) {
	tm.mu.Lock()
	entry, ok := tm.tabs[tabID]
	if !ok || entry.autoCloseGen != gen {
		tm.mu.Unlock()
		return
	}
	entry.autoCloseTimer = nil
	tm.mu.Unlock()

	if err := tm.CloseTab(tabID); err != nil {
		slog.Debug("auto-close tab failed", "tabId", tabID, "err", err)
		return
	}
	slog.Info("tab auto-closed", "tabId", tabID, "reason", "auto_close")
}
