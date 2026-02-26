package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	cdp "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

type TabSetupFunc func(ctx context.Context)

type TabManager struct {
	browserCtx context.Context
	config     *config.RuntimeConfig
	tabs       map[string]*TabEntry
	accessed   map[string]bool
	snapshots  map[string]*RefCache
	onTabSetup TabSetupFunc
	mu         sync.RWMutex
}

func NewTabManager(browserCtx context.Context, cfg *config.RuntimeConfig, onTabSetup TabSetupFunc) *TabManager {
	return &TabManager{
		browserCtx: browserCtx,
		config:     cfg,
		tabs:       make(map[string]*TabEntry),
		accessed:   make(map[string]bool),
		snapshots:  make(map[string]*RefCache),
		onTabSetup: onTabSetup,
	}
}

func (tm *TabManager) markAccessed(tabID string) {
	tm.mu.Lock()
	tm.accessed[tabID] = true
	tm.mu.Unlock()
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
	if entry, ok := tm.tabs[tabID]; ok && entry.Ctx != nil {
		tm.mu.RUnlock()
		tm.markAccessed(tabID)
		return entry.Ctx, tabID, nil
	}
	tm.mu.RUnlock()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry, ok := tm.tabs[tabID]; ok && entry.Ctx != nil {
		tm.accessed[tabID] = true
		return entry.Ctx, tabID, nil
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

	tm.tabs[tabID] = &TabEntry{Ctx: ctx, Cancel: cancel}
	return ctx, tabID, nil
}

func (tm *TabManager) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	if tm.browserCtx == nil {
		return "", nil, nil, fmt.Errorf("no browser context available")
	}

	if tm.config.MaxTabs > 0 {
		targets, err := tm.ListTargets()
		if err != nil {
			return "", nil, nil, fmt.Errorf("check tab count: %w", err)
		}
		if len(targets) >= tm.config.MaxTabs {
			return "", nil, nil, fmt.Errorf("tab limit reached (%d/%d) — close a tab first", len(targets), tm.config.MaxTabs)
		}
	}

	// Use target.CreateTarget CDP protocol call to create a new tab.
	// This works for both local and remote (CDP_URL) allocators.
	navURL := "about:blank"
	if url != "" {
		navURL = url
	}

	var targetID target.ID
	createCtx, createCancel := context.WithTimeout(tm.browserCtx, 10*time.Second)
	if err := chromedp.Run(createCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			targetID, err = target.CreateTarget(navURL).Do(ctx)
			return err
		}),
	); err != nil {
		createCancel()
		return "", nil, nil, fmt.Errorf("create target: %w", err)
	}
	createCancel()

	// Create a context for the new tab
	ctx, cancel := chromedp.NewContext(tm.browserCtx,
		chromedp.WithTargetID(targetID),
	)

	if tm.onTabSetup != nil {
		tm.onTabSetup(ctx)
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

	if len(blockPatterns) > 0 {
		_ = SetResourceBlocking(ctx, blockPatterns)
	}

	newTargetID := string(targetID)
	tm.mu.Lock()
	tm.tabs[newTargetID] = &TabEntry{Ctx: ctx, Cancel: cancel}
	tm.accessed[newTargetID] = true
	tm.mu.Unlock()

	return newTargetID, ctx, cancel, nil
}

func (tm *TabManager) CloseTab(tabID string) error {
	tm.mu.Lock()
	entry, tracked := tm.tabs[tabID]
	tm.mu.Unlock()

	if tracked && entry.Cancel != nil {
		entry.Cancel()
	}

	closeCtx, closeCancel := context.WithTimeout(tm.browserCtx, 5*time.Second)
	defer closeCancel()

	if err := target.CloseTarget(target.ID(tabID)).Do(cdp.WithExecutor(closeCtx, chromedp.FromContext(closeCtx).Browser)); err != nil {
		if !tracked {
			return fmt.Errorf("tab %s not found", tabID)
		}
		slog.Debug("close target CDP", "tabId", tabID, "err", err)
	}

	tm.mu.Lock()
	delete(tm.tabs, tabID)
	delete(tm.snapshots, tabID)
	tm.mu.Unlock()

	return nil
}

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
		if t.Type == TargetTypePage {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

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

func (tm *TabManager) RegisterTab(tabID string, ctx context.Context) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx}
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

		tm.mu.Lock()
		for id, entry := range tm.tabs {
			if !alive[id] {
				if entry.Cancel != nil {
					entry.Cancel()
				}
				delete(tm.tabs, id)
				delete(tm.snapshots, id)
				slog.Info("cleaned stale tab", "id", id)
			}
		}
		tm.mu.Unlock()
	}
}
