package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

func (tm *TabManager) markAccessed(tabID string) {
	tm.mu.Lock()
	tm.accessed[tabID] = true
	if entry, ok := tm.tabs[tabID]; ok {
		entry.LastUsed = time.Now()
	}
	tm.currentTab = tabID
	tm.mu.Unlock()
}

// selectCurrentTrackedTab returns the current tab ID, falling back to the most
// recently used tab if the explicit pointer is stale or unset.
func (tm *TabManager) selectCurrentTrackedTab() string {
	if tm.currentTab != "" {
		if _, ok := tm.tabs[tm.currentTab]; ok {
			return tm.currentTab
		}
	}

	var best string
	var bestTime time.Time
	for id, entry := range tm.tabs {
		if entry.LastUsed.After(bestTime) {
			best = id
			bestTime = entry.LastUsed
		}
	}
	if best == "" {
		for id, entry := range tm.tabs {
			if entry.CreatedAt.After(bestTime) {
				best = id
				bestTime = entry.CreatedAt
			}
		}
	}
	return best
}

// AccessedTabIDs returns the set of tab IDs that were accessed this session.
func (tm *TabManager) AccessedTabIDs() map[string]bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make(map[string]bool, len(tm.accessed))
	for k := range tm.accessed {
		out[k] = true
	}
	return out
}

func (tm *TabManager) TabContext(tabID string) (context.Context, string, error) {
	if tm == nil {
		return nil, "", fmt.Errorf("tab manager not initialized")
	}
	if tabID == "" {
		tm.mu.RLock()
		tabID = tm.selectCurrentTrackedTab()
		tm.mu.RUnlock()

		if tabID == "" {
			targets, err := tm.ListTargets()
			if err != nil {
				return nil, "", fmt.Errorf("list targets: %w", err)
			}
			if len(targets) == 0 {
				return nil, "", fmt.Errorf("no tabs open")
			}
			rawID := string(targets[0].TargetID)
			tabID = tm.idMgr.TabIDFromCDPTarget(rawID)
		}
	}

	tm.mu.RLock()
	entry, ok := tm.tabs[tabID]
	tm.mu.RUnlock()

	if !ok {
		targets, err := tm.ListTargets()
		if err == nil {
			for _, t := range targets {
				raw := string(t.TargetID)
				if tm.idMgr.TabIDFromCDPTarget(raw) == tabID {
					ctx, cancel := chromedp.NewContext(tm.browserCtx, chromedp.WithTargetID(target.ID(raw)))
					if tm.onTabSetup != nil {
						tm.onTabSetup(ctx)
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
					tm.RegisterTabWithCancel(tabID, raw, ctx, cancel)

					tm.mu.RLock()
					entry = tm.tabs[tabID]
					tm.mu.RUnlock()
					ok = true
					break
				}
			}
		}
	}

	if !ok {
		return nil, "", fmt.Errorf("tab %s not found", tabID)
	}

	if entry.Ctx == nil {
		return nil, "", fmt.Errorf("tab %s has no active context", tabID)
	}

	tm.markAccessed(tabID)

	return entry.Ctx, tabID, nil
}

// ResolveTabByIndex resolves a 1-based tab index to a tab ID.
func (tm *TabManager) ResolveTabByIndex(index int) (string, string, string, error) {
	targets, err := tm.ListTargets()
	if err != nil {
		return "", "", "", err
	}
	if index < 1 || index > len(targets) {
		return "", "", "", fmt.Errorf("tab index %d out of range (1-%d)", index, len(targets))
	}
	t := targets[index-1]
	tabID := tm.idMgr.TabIDFromCDPTarget(string(t.TargetID))
	return tabID, t.URL, t.Title, nil
}

func (tm *TabManager) ListTargets() ([]*target.Info, error) {
	if tm == nil {
		return nil, fmt.Errorf("tab manager not initialized")
	}
	if tm.browserCtx == nil {
		return nil, fmt.Errorf("no browser connection")
	}

	execCtx, err := browserExecutorContext(tm.browserCtx)
	if err != nil {
		return nil, fmt.Errorf("browser executor: %w", err)
	}

	ctx, cancel := context.WithTimeout(execCtx, 5*time.Second)
	defer cancel()

	targets, err := target.GetTargets().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("get targets: %w", err)
	}

	pages := make([]*target.Info, 0, len(targets))
	for _, t := range targets {
		if t.Type == TargetTypePage {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// ListTargetsWithContext is like ListTargets but uses a custom context for timeout control.
func (tm *TabManager) ListTargetsWithContext(ctx context.Context) ([]*target.Info, error) {
	if tm == nil {
		return nil, fmt.Errorf("tab manager not initialized")
	}
	if tm.browserCtx == nil {
		return nil, fmt.Errorf("no browser connection")
	}

	execCtx, err := browserExecutorContext(tm.browserCtx)
	if err != nil {
		return nil, fmt.Errorf("browser executor: %w", err)
	}

	mergedCtx, cancel := context.WithCancel(execCtx)
	defer cancel()

	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-mergedCtx.Done():
		}
	}()

	targets, err := target.GetTargets().Do(mergedCtx)
	if err != nil {
		return nil, fmt.Errorf("get targets: %w", err)
	}

	pages := make([]*target.Info, 0, len(targets))
	for _, t := range targets {
		if t.Type == TargetTypePage {
			pages = append(pages, t)
		}
	}
	return pages, nil
}
