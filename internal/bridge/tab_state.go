package bridge

import (
	"context"
	"time"
)

func (tm *TabManager) GetRefCache(tabID string) *RefCache {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.snapshots[tabID]
}

func (tm *TabManager) SetRefCache(tabID string, cache *RefCache) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.snapshots[tabID] = cache
}

func (tm *TabManager) DeleteRefCache(tabID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.snapshots, tabID)
}

func (tm *TabManager) GetFrameScope(tabID string) (FrameScope, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	scope, ok := tm.frameScope[tabID]
	return scope, ok && scope.Active()
}

func (tm *TabManager) SetFrameScope(tabID string, scope FrameScope) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !scope.Active() {
		delete(tm.frameScope, tabID)
		return
	}
	tm.frameScope[tabID] = scope
}

func (tm *TabManager) ClearFrameScope(tabID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.frameScope, tabID)
}

func (tm *TabManager) RegisterTab(tabID string, ctx context.Context) {
	now := time.Now()
	tm.mu.Lock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx, CreatedAt: now, LastUsed: now}
	tm.currentTab = tabID
	tm.mu.Unlock()

	tm.startTabPolicyWatcher(tabID, ctx)
}

// RegisterTabWithCancel registers a tab ID with its context and cancel function.
func (tm *TabManager) RegisterTabWithCancel(tabID, rawCDPID string, ctx context.Context, cancel context.CancelFunc) {
	now := time.Now()
	tm.mu.Lock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx, Cancel: cancel, CDPID: rawCDPID, CreatedAt: now, LastUsed: now}
	tm.currentTab = tabID
	tm.mu.Unlock()

	tm.startTabPolicyWatcher(tabID, ctx)
}
