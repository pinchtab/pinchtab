package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/ids"
)

func (tm *TabManager) browserGuardsActive() bool {
	if tm == nil {
		return false
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.guardActive
}

func (tm *TabManager) tabBlockPatterns() []string {
	if tm == nil || tm.config == nil {
		return nil
	}
	var blockPatterns []string
	if tm.config.BlockAds {
		blockPatterns = CombineBlockPatterns(blockPatterns, AdBlockPatterns)
	}
	if tm.config.BlockMedia {
		blockPatterns = CombineBlockPatterns(blockPatterns, MediaBlockPatterns)
	} else if tm.config.BlockImages {
		blockPatterns = CombineBlockPatterns(blockPatterns, ImageBlockPatterns)
	}
	return blockPatterns
}

func (tm *TabManager) setupManagedTarget(ctx context.Context, tabID, rawCDPID string) bool {
	if tm.onTabSetup != nil {
		tm.onTabSetup(ctx)
	}
	if blockPatterns := tm.tabBlockPatterns(); len(blockPatterns) > 0 {
		if err := SetResourceBlocking(ctx, blockPatterns); err != nil {
			slog.Warn("resource blocking setup failed", "tabId", tabID, "err", err)
		}
	}
	if tm.netMonitor != nil {
		if err := tm.netMonitor.StartCapture(ctx, tabID); err != nil {
			slog.Warn("eager network capture failed", "tab", tabID, "err", err)
		}
	}
	if tm.dialogMgr != nil {
		autoAccept := tm.config != nil && tm.config.DialogAutoAccept
		ListenDialogEvents(ctx, tabID, tm.dialogMgr, autoAccept)
		if err := EnableDialogEvents(ctx); err != nil {
			slog.Warn("enable dialog events failed", "tabId", tabID, "err", err)
		}
	}
	consoleEnabled := tm.shouldEagerlyCaptureConsole()
	if consoleEnabled {
		tm.setupConsoleCapture(ctx, rawCDPID)
	}
	return consoleEnabled
}

func (tm *TabManager) enforceAdoptTabLimit() error {
	if tm == nil || tm.config == nil || tm.config.MaxTabs <= 0 {
		return nil
	}
	tm.mu.RLock()
	managedCount := len(tm.tabs)
	tm.mu.RUnlock()
	if managedCount < tm.config.MaxTabs {
		return nil
	}
	switch tm.config.TabEvictionPolicy {
	case "close_oldest":
		if err := tm.closeOldestTab(); err != nil {
			return fmt.Errorf("eviction failed: %w", err)
		}
	case "reject":
		return &TabLimitError{Current: managedCount, Max: tm.config.MaxTabs}
	default:
		if err := tm.closeLRUTab(); err != nil {
			return fmt.Errorf("eviction failed: %w", err)
		}
	}
	return nil
}

func (tm *TabManager) adoptExistingTarget(targetID target.ID, enforceLimit bool) (string, error) {
	if tm == nil {
		return "", fmt.Errorf("tab manager not initialized")
	}
	if tm.browserCtx == nil {
		return "", fmt.Errorf("no browser context available")
	}
	rawCDPID := string(targetID)
	if rawCDPID == "" {
		return "", fmt.Errorf("target id required")
	}
	if tm.idMgr == nil {
		tm.idMgr = ids.NewManager()
	}
	tabID := tm.idMgr.TabIDFromCDPTarget(rawCDPID)

	tm.mu.RLock()
	entry, tracked := tm.tabs[tabID]
	tm.mu.RUnlock()
	if tracked {
		if entry == nil || entry.Ctx == nil {
			return "", fmt.Errorf("tab %s has no active context", tabID)
		}
		tm.markAccessed(tabID)
		return tabID, nil
	}

	if enforceLimit {
		if err := tm.enforceAdoptTabLimit(); err != nil {
			return "", err
		}
	}

	ctx, cancel := chromedp.NewContext(tm.browserCtx, chromedp.WithTargetID(targetID))
	consoleEnabled := tm.setupManagedTarget(ctx, tabID, rawCDPID)
	now := time.Now()

	tm.mu.Lock()
	tm.tabs[tabID] = &TabEntry{
		Ctx:                   ctx,
		Cancel:                cancel,
		CDPID:                 rawCDPID,
		CreatedAt:             now,
		LastUsed:              now,
		ConsoleCaptureEnabled: consoleEnabled,
	}
	tm.accessed[tabID] = true
	tm.currentTab = tabID
	tm.mu.Unlock()

	tm.startTabPolicyWatcher(tabID, ctx)
	return tabID, nil
}
