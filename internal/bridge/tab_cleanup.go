package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// closeOldestTab evicts the tab with the earliest CreatedAt timestamp.
func (tm *TabManager) closeOldestTab() error {
	tm.mu.RLock()
	var oldestID string
	var oldestTime time.Time
	for id, entry := range tm.tabs {
		if oldestID == "" || entry.CreatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = entry.CreatedAt
		}
	}
	tm.mu.RUnlock()

	if oldestID == "" {
		return fmt.Errorf("no tabs to evict")
	}
	slog.Info("evicting oldest tab", "id", oldestID, "createdAt", oldestTime)
	return tm.CloseTab(oldestID)
}

// closeLRUTab evicts the tab with the earliest LastUsed timestamp.
func (tm *TabManager) closeLRUTab() error {
	tm.mu.RLock()
	var lruID string
	var lruTime time.Time
	for id, entry := range tm.tabs {
		t := entry.LastUsed
		if t.IsZero() {
			t = entry.CreatedAt
		}
		if lruID == "" || t.Before(lruTime) {
			lruID = id
			lruTime = t
		}
	}
	tm.mu.RUnlock()

	if lruID == "" {
		return fmt.Errorf("no tabs to evict")
	}
	slog.Info("evicting LRU tab", "id", lruID, "lastUsed", lruTime)
	return tm.CloseTab(lruID)
}

func (tm *TabManager) CleanStaleTabs(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		targets, err := tm.ListTargets()
		if err != nil {
			continue
		}

		alive := make(map[string]bool, len(targets))
		for _, t := range targets {
			alive[string(t.TargetID)] = true
		}

		type staleTab struct {
			tabID string
			cdpID string
		}
		var staleTabs []staleTab
		tm.mu.RLock()
		for id, entry := range tm.tabs {
			if !alive[id] {
				cdpID := entry.CDPID
				if cdpID == "" {
					cdpID = id
				}
				staleTabs = append(staleTabs, staleTab{tabID: id, cdpID: cdpID})
			}
		}
		tm.mu.RUnlock()

		for _, stale := range staleTabs {
			tm.purgeTrackedTabState(stale.tabID, stale.cdpID)
			slog.Info("cleaned stale tab", "id", stale.tabID)
		}
	}
}

func (tm *TabManager) purgeTrackedTabState(tabID, cdpTargetID string) bool {
	resolvedTabID, resolvedCDPID, cancel, ok := tm.lookupTrackedTabForCleanup(tabID, cdpTargetID)
	if !ok {
		return false
	}
	if cancel != nil {
		cancel()
	}

	tm.mu.Lock()
	if entry, ok := tm.tabs[resolvedTabID]; ok && entry.autoCloseTimer != nil {
		entry.autoCloseTimer.Stop()
		entry.autoCloseTimer = nil
		entry.autoCloseGen++
	}
	delete(tm.tabs, resolvedTabID)
	delete(tm.snapshots, resolvedTabID)
	delete(tm.frameScope, resolvedTabID)
	delete(tm.accessed, resolvedTabID)
	if tm.currentTab == resolvedTabID {
		tm.currentTab = ""
	}
	tm.mu.Unlock()

	if tm.dialogMgr != nil {
		tm.dialogMgr.ClearPending(resolvedTabID)
	}
	if tm.executor != nil {
		tm.executor.RemoveTab(resolvedTabID)
	}
	if tm.logStore != nil {
		tm.logStore.RemoveTab(resolvedCDPID)
	}
	// Notify listeners (e.g. session persistence) that a tab disappeared,
	// regardless of whether the trigger was a deliberate CloseTab, an eviction,
	// the auto-close lifecycle timer, or Chrome reporting the target gone
	// (CleanStaleTabs / user closing the tab in headed Chrome directly).
	if tm.onAfterClose != nil {
		tm.onAfterClose()
	}
	return true
}

func (tm *TabManager) lookupTrackedTabForCleanup(tabID, cdpTargetID string) (string, string, context.CancelFunc, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tabID != "" {
		if entry, ok := tm.tabs[tabID]; ok {
			resolvedCDPID := cdpTargetID
			if resolvedCDPID == "" {
				resolvedCDPID = entry.CDPID
			}
			if resolvedCDPID == "" {
				resolvedCDPID = tabID
			}
			return tabID, resolvedCDPID, entry.Cancel, true
		}
	}

	if cdpTargetID == "" {
		return "", "", nil, false
	}

	for id, entry := range tm.tabs {
		resolvedCDPID := entry.CDPID
		if resolvedCDPID == "" {
			resolvedCDPID = id
		}
		if id == cdpTargetID || resolvedCDPID == cdpTargetID {
			return id, resolvedCDPID, entry.Cancel, true
		}
	}
	return "", "", nil, false
}

func (tm *TabManager) purgeTrackedTabStateByTargetID(cdpTargetID string) bool {
	return tm.purgeTrackedTabState("", cdpTargetID)
}
