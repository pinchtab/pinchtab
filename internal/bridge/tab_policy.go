package bridge

import (
	"context"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/idpi"
)

type TabPolicyState struct {
	CurrentURL string
	Threat     bool
	Blocked    bool
	Reason     string
	UpdatedAt  time.Time
}

func EvaluateTabPolicy(rawURL string, cfg config.IDPIConfig) TabPolicyState {
	result := idpi.CheckDomain(rawURL, cfg)
	return TabPolicyState{
		CurrentURL: rawURL,
		Threat:     result.Threat,
		Blocked:    result.Blocked,
		Reason:     result.Reason,
		UpdatedAt:  time.Now(),
	}
}

func (tm *TabManager) idpiDomainPolicyActive() bool {
	return tm != nil &&
		tm.config != nil &&
		tm.config.IDPI.Enabled &&
		len(tm.config.IDPI.AllowedDomains) > 0
}

func (tm *TabManager) startTabPolicyWatcher(tabID string, ctx context.Context) {
	if !tm.idpiDomainPolicyActive() || ctx == nil {
		return
	}

	tm.mu.Lock()
	entry := tm.tabs[tabID]
	if entry == nil || entry.Watching {
		tm.mu.Unlock()
		return
	}
	entry.Watching = true
	tm.mu.Unlock()

	tm.refreshTabPolicyFromContext(tabID, ctx)

	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *page.EventFrameNavigated:
			if e.Frame == nil || e.Frame.ParentID != "" {
				return
			}
			tm.updateTabPolicy(tabID, e.Frame.URL)
		case *page.EventNavigatedWithinDocument:
			go tm.refreshTabPolicyFromContext(tabID, ctx)
		}
	})
}

func (tm *TabManager) refreshTabPolicyFromContext(tabID string, ctx context.Context) {
	if !tm.idpiDomainPolicyActive() || ctx == nil {
		return
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var currentURL string
	if err := chromedp.Run(lookupCtx, chromedp.Location(&currentURL)); err != nil {
		slog.Debug("tab policy refresh failed", "tabId", tabID, "err", err)
		return
	}

	tm.updateTabPolicy(tabID, currentURL)
}

func (tm *TabManager) updateTabPolicy(tabID, rawURL string) TabPolicyState {
	state := EvaluateTabPolicy(rawURL, tm.config.IDPI)

	tm.mu.Lock()
	if entry := tm.tabs[tabID]; entry != nil {
		entry.Policy = state
	}
	tm.mu.Unlock()

	if state.Threat {
		level := slog.LevelWarn
		if !state.Blocked {
			level = slog.LevelInfo
		}
		slog.Log(context.Background(), level, "tab domain policy updated",
			"tabId", tabID,
			"url", state.CurrentURL,
			"blocked", state.Blocked,
			"reason", state.Reason,
		)
	}

	return state
}

func (tm *TabManager) GetTabPolicyState(tabID string) (TabPolicyState, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	entry := tm.tabs[tabID]
	if entry == nil || entry.Policy.UpdatedAt.IsZero() {
		return TabPolicyState{}, false
	}
	return entry.Policy, true
}

func (tm *TabManager) SetTabPolicyState(tabID string, state TabPolicyState) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry := tm.tabs[tabID]; entry != nil {
		entry.Policy = state
	}
}
