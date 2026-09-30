package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

const lifecycleCallTimeout = 5 * time.Second

type TabUnfreezeError struct {
	TabID string
	Err   error
}

func (e *TabUnfreezeError) Error() string {
	return fmt.Sprintf("tab %s could not be unfrozen: %v", e.TabID, e.Err)
}

func (e *TabUnfreezeError) Unwrap() error { return e.Err }

func (tm *TabManager) SetLifecycleWriterForTests(write func(ctx context.Context, frozen bool) error) {
	tm.setFrozen = write
}

func setTabFrozen(ctx context.Context, frozen bool) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return bridgecdpops.SetPageFrozen(c, frozen)
	}))
}

func (tm *TabManager) applyFrozen(tabCtx context.Context, frozen bool) error {
	ctx, cancel := context.WithTimeout(tabCtx, lifecycleCallTimeout)
	defer cancel()
	return tm.setFrozen(ctx, frozen)
}

func (tm *TabManager) SetFreezeVeto(veto func(tabID string) bool) {
	tm.freezeVeto = veto
}

func (tm *TabManager) keepsAwake(tabID string) bool {
	if tm.freezeVeto != nil && tm.freezeVeto(tabID) {
		return true
	}
	return len(tm.routeMgr.List(tabID)) > 0
}

func (tm *TabManager) HoldAwake(tabID string) (release func()) {
	tm.mu.Lock()
	entry, ok := tm.tabs[tabID]
	if ok {
		entry.awakeHolds++
	}
	tm.mu.Unlock()
	if !ok {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			tm.mu.Lock()
			entry.awakeHolds--
			idle := entry.awakeHolds == 0
			tm.mu.Unlock()
			if idle {
				tm.rearmFreeze(tabID)
			}
		})
	}
}

func (tm *TabManager) HoldAwakeUntil(ctx context.Context, tabID string) {
	if tm == nil || !tm.freezesIdleTabs() {
		return
	}
	context.AfterFunc(ctx, tm.HoldAwake(tabID))
}

func (tm *TabManager) TabFrozen(tabID string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry, ok := tm.tabs[tabID]
	return ok && entry.frozen
}

func (tm *TabManager) rearmFreeze(tabID string) {
	if tm.freezesIdleTabs() {
		tm.ScheduleIdleLifecycle(tabID)
	}
}

func (tm *TabManager) freezeIdleTab(tabID string, gen uint64) {
	if tm.keepsAwake(tabID) {
		return
	}
	tm.mu.RLock()
	entry, ok := tm.tabs[tabID]
	tm.mu.RUnlock()
	if !ok {
		return
	}
	entry.lifecycleMu.Lock()
	defer entry.lifecycleMu.Unlock()

	tm.mu.Lock()
	if entry.idleGen != gen || entry.Ctx == nil || entry.awakeHolds > 0 {
		tm.mu.Unlock()
		return
	}
	entry.idleTimer = nil
	entry.frozen = true
	ctx := entry.Ctx
	tm.mu.Unlock()

	if err := tm.applyFrozen(ctx, true); err != nil {
		slog.Debug("freeze idle tab failed", "tabId", tabID, "err", err)
		return
	}
	slog.Info("tab frozen", "tabId", tabID, "reason", "freeze_idle")
}

func (tm *TabManager) thawTab(tabID string, entry *TabEntry) error {
	entry.lifecycleMu.Lock()
	defer entry.lifecycleMu.Unlock()

	tm.mu.RLock()
	frozen, ctx := entry.frozen, entry.Ctx
	tm.mu.RUnlock()
	if !frozen {
		return nil
	}
	if err := tm.applyFrozen(ctx, false); err != nil {
		return &TabUnfreezeError{TabID: tabID, Err: err}
	}
	tm.mu.Lock()
	entry.frozen = false
	tm.mu.Unlock()
	return nil
}

func (tm *TabManager) touchTab(tabID string) error {
	tm.mu.Lock()
	tm.accessed[tabID] = true
	entry, ok := tm.tabs[tabID]
	if ok {
		entry.LastUsed = time.Now()
	}
	tm.mu.Unlock()
	if !ok {
		return nil
	}
	tm.rearmFreeze(tabID)
	return tm.thawTab(tabID, entry)
}
