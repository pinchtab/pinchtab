package bridge

import (
	"context"
	"log/slog"
	"time"

	cdp "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"github.com/pinchtab/pinchtab/internal/readiness"
)

func (tm *TabManager) trackedCDPIDs() map[string]bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make(map[string]bool, len(tm.tabs))
	for _, entry := range tm.tabs {
		if entry != nil && entry.CDPID != "" {
			out[entry.CDPID] = true
		}
	}
	return out
}

func selectOrphanTargets(tracked map[string]bool, pages []*target.Info, limit int) []target.ID {
	orphans := make([]target.ID, 0)
	for i := len(pages) - 1; i >= 0; i-- {
		t := pages[i]
		if t == nil || t.Type != TargetTypePage {
			continue
		}
		if tracked[string(t.TargetID)] {
			continue
		}
		orphans = append(orphans, t.TargetID)
		if limit > 0 && len(orphans) >= limit {
			break
		}
	}
	return orphans
}

func (tm *TabManager) closeTarget(id target.ID) bool {
	closeCtx, cancel := context.WithTimeout(tm.browserCtx, 5*time.Second)
	defer cancel()
	c := chromedp.FromContext(closeCtx)
	if c == nil || c.Browser == nil {
		return false
	}
	if err := target.CloseTarget(id).Do(cdp.WithExecutor(closeCtx, c.Browser)); err != nil {
		slog.Debug("tab budget: orphan close failed", "targetId", id, "err", err)
		return false
	}
	return true
}

func (tm *TabManager) reconcileTabBudget() {
	if tm == nil || tm.config == nil || tm.config.MaxTabs <= 0 || tm.browserCtx == nil {
		return
	}

	pages, err := tm.ListTargets()
	if err != nil {
		slog.Debug("tab budget: launch reconcile skipped", "err", err)
		return
	}
	if len(pages) <= tm.config.MaxTabs {
		return
	}

	orphans := selectOrphanTargets(tm.trackedCDPIDs(), pages, len(pages)-tm.config.MaxTabs)
	closed := 0
	for _, id := range orphans {
		if !tm.closeTarget(id) {
			continue
		}
		closed++
	}
	if closed == 0 {
		return
	}

	live := len(pages)
	_, _ = readiness.WaitUntil(context.Background(), 5*time.Second, 25*time.Millisecond, func() (struct{}, bool, error) {
		remaining, err := tm.ListTargets()
		if err != nil {
			return struct{}{}, false, err
		}
		live = len(remaining)
		return struct{}{}, live <= tm.config.MaxTabs, nil
	})

	slog.Info("tab budget: reaped untracked targets on browser launch",
		"closed", closed, "live", live, "maxTabs", tm.config.MaxTabs)
}
