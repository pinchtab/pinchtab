package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	cdp "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// TabSetupFunc is called after a new tab context is created (stealth, animations, etc.).
type TabSetupFunc func(ctx context.Context)

// TabManager manages browser tabs, their contexts, and ref caches.
type TabManager struct {
	browserCtx context.Context
	tabs       map[string]*TabEntry
	snapshots  map[string]*refCache
	onTabSetup TabSetupFunc
	mu         sync.RWMutex
}

// NewTabManager creates a TabManager with the given browser context and setup hook.
func NewTabManager(browserCtx context.Context, onTabSetup TabSetupFunc) *TabManager {
	return &TabManager{
		browserCtx: browserCtx,
		tabs:       make(map[string]*TabEntry),
		snapshots:  make(map[string]*refCache),
		onTabSetup: onTabSetup,
	}
}

// TabContext returns or creates a context for the given tab ID.
// If tabID is empty, the first available page target is used.
func (tm *TabManager) TabContext(tabID string) (context.Context, string, error) {
	if tabID == "" {
		targets, err := tm.ListTargets()
		if err != nil {
			return nil, "", fmt.Errorf("list targets: %w", err)
		}
		if len(targets) == 0 {
			return nil, "", fmt.Errorf("no tabs open")
		}
		tabID = string(targets[0].TargetID)
	}

	tm.mu.RLock()
	if entry, ok := tm.tabs[tabID]; ok && entry.ctx != nil {
		tm.mu.RUnlock()
		return entry.ctx, tabID, nil
	}
	tm.mu.RUnlock()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry, ok := tm.tabs[tabID]; ok && entry.ctx != nil {
		return entry.ctx, tabID, nil
	}

	if tm.browserCtx == nil {
		return nil, "", fmt.Errorf("no browser connection")
	}

	ctx, cancel := chromedp.NewContext(tm.browserCtx,
		chromedp.WithTargetID(target.ID(tabID)),
	)
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return nil, "", fmt.Errorf("tab %s not found: %w", tabID, err)
	}

	if tm.onTabSetup != nil {
		tm.onTabSetup(ctx)
	}

	tm.tabs[tabID] = &TabEntry{ctx: ctx, cancel: cancel}
	return ctx, tabID, nil
}

// CreateTab opens a new tab, optionally navigating to url.
func (tm *TabManager) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	if tm.browserCtx == nil {
		return "", nil, nil, fmt.Errorf("no browser context available")
	}
	ctx, cancel := chromedp.NewContext(tm.browserCtx)

	if tm.onTabSetup != nil {
		tm.onTabSetup(ctx)
	}

	if cfg.BlockMedia {
		_ = setResourceBlocking(ctx, mediaBlockPatterns)
	} else if cfg.BlockImages {
		_ = setResourceBlocking(ctx, imageBlockPatterns)
	}

	navURL := "about:blank"
	if url != "" {
		navURL = url
	}
	if err := navigatePage(ctx, navURL); err != nil {
		cancel()
		return "", nil, nil, fmt.Errorf("new tab: %w", err)
	}

	newTargetID := string(chromedp.FromContext(ctx).Target.TargetID)
	tm.mu.Lock()
	tm.tabs[newTargetID] = &TabEntry{ctx: ctx, cancel: cancel}
	tm.mu.Unlock()

	return newTargetID, ctx, cancel, nil
}

// CloseTab closes a tab by ID.
func (tm *TabManager) CloseTab(tabID string) error {
	tm.mu.Lock()
	entry, tracked := tm.tabs[tabID]
	tm.mu.Unlock()

	if tracked && entry.cancel != nil {
		entry.cancel()
	}

	closeCtx, closeCancel := context.WithTimeout(tm.browserCtx, 5*time.Second)
	defer closeCancel()

	if err := target.CloseTarget(target.ID(tabID)).Do(cdp.WithExecutor(closeCtx, chromedp.FromContext(closeCtx).Browser)); err != nil {
		if !tracked {
			return nil
		}
		slog.Debug("close target CDP", "tabId", tabID, "err", err)
	}

	tm.mu.Lock()
	delete(tm.tabs, tabID)
	delete(tm.snapshots, tabID)
	tm.mu.Unlock()

	return nil
}

// ListTargets returns all page-type targets from the browser.
func (tm *TabManager) ListTargets() ([]*target.Info, error) {
	if tm.browserCtx == nil {
		return nil, fmt.Errorf("no browser connection")
	}
	var targets []*target.Info
	if err := chromedp.Run(tm.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			targets, err = target.GetTargets().Do(ctx)
			return err
		}),
	); err != nil {
		return nil, fmt.Errorf("get targets: %w", err)
	}

	pages := make([]*target.Info, 0)
	for _, t := range targets {
		if t.Type == targetTypePage {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// GetRefCache returns the ref cache for a tab.
func (tm *TabManager) GetRefCache(tabID string) *refCache {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.snapshots[tabID]
}

// SetRefCache sets the ref cache for a tab.
func (tm *TabManager) SetRefCache(tabID string, cache *refCache) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.snapshots[tabID] = cache
}

// DeleteRefCache removes the ref cache for a tab.
func (tm *TabManager) DeleteRefCache(tabID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.snapshots, tabID)
}

// RegisterTab registers an externally-created tab (e.g., the initial Chrome tab).
func (tm *TabManager) RegisterTab(tabID string, ctx context.Context) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tabs[tabID] = &TabEntry{ctx: ctx}
}

// CleanStaleTabs periodically removes tabs that no longer exist in Chrome.
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

		tm.mu.Lock()
		for id, entry := range tm.tabs {
			if !alive[id] {
				if entry.cancel != nil {
					entry.cancel()
				}
				delete(tm.tabs, id)
				delete(tm.snapshots, id)
				slog.Info("cleaned stale tab", "id", id)
			}
		}
		tm.mu.Unlock()
	}
}
