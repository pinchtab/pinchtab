package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	cdp "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// TabEntry holds a chromedp context for an open tab.
type TabEntry struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// refCache stores the ref→backendNodeID mapping and the last snapshot nodes per tab.
// Refs are assigned during /snapshot and looked up during /action, avoiding
// a second a11y tree fetch that could drift.
type refCache struct {
	refs  map[string]int64 // "e0" → backendNodeID
	nodes []A11yNode       // last snapshot for diff
}

// Bridge is the central state holder for the Chrome connection, tab contexts,
// and per-tab snapshot caches.
type Bridge struct {
	allocCtx      context.Context
	browserCtx    context.Context
	tabs          map[string]*TabEntry
	snapshots     map[string]*refCache
	stealthScript string // injected on every new tab
	mu            sync.RWMutex
}

// injectStealth adds the stealth script to a tab context so it runs on every
// new document load (including navigations).
func (b *Bridge) injectStealth(ctx context.Context) {
	if b.stealthScript == "" {
		return
	}
	_ = chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(b.stealthScript).Do(ctx)
			return err
		}),
	)
}

// TabContext returns the chromedp context for a tab and the resolved tabID.
// If tabID is empty, uses the first page target.
// Uses RLock for cache hits, upgrades to Lock only when creating a new entry.
func (b *Bridge) TabContext(tabID string) (context.Context, string, error) {
	if tabID == "" {
		targets, err := b.ListTargets()
		if err != nil {
			return nil, "", fmt.Errorf("list targets: %w", err)
		}
		if len(targets) == 0 {
			return nil, "", fmt.Errorf("no tabs open")
		}
		tabID = string(targets[0].TargetID)
	}

	// Fast path: read lock
	b.mu.RLock()
	if entry, ok := b.tabs[tabID]; ok && entry.ctx != nil {
		b.mu.RUnlock()
		return entry.ctx, tabID, nil
	}
	b.mu.RUnlock()

	// Slow path: write lock, double-check
	b.mu.Lock()
	defer b.mu.Unlock()

	if entry, ok := b.tabs[tabID]; ok && entry.ctx != nil {
		return entry.ctx, tabID, nil
	}

	if b.browserCtx == nil {
		return nil, "", fmt.Errorf("no browser connection")
	}

	ctx, cancel := chromedp.NewContext(b.browserCtx,
		chromedp.WithTargetID(target.ID(tabID)),
	)
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return nil, "", fmt.Errorf("tab %s not found: %w", tabID, err)
	}

	// Inject stealth script on newly attached tabs
	b.injectStealth(ctx)

	b.tabs[tabID] = &TabEntry{ctx: ctx, cancel: cancel}
	return ctx, tabID, nil
}

// CleanStaleTabs periodically removes tab entries whose Chrome targets
// no longer exist. Exits when ctx is cancelled.
func (b *Bridge) CleanStaleTabs(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		targets, err := b.ListTargets()
		if err != nil {
			continue
		}

		alive := make(map[string]bool, len(targets))
		for _, t := range targets {
			alive[string(t.TargetID)] = true
		}

		b.mu.Lock()
		for id, entry := range b.tabs {
			if !alive[id] {
				if entry.cancel != nil {
					entry.cancel()
				}
				delete(b.tabs, id)
				delete(b.snapshots, id)
				slog.Info("cleaned stale tab", "id", id)
			}
		}
		b.mu.Unlock()
	}
}

// GetRefCache returns the cached snapshot refs for a tab (nil if none).
func (b *Bridge) GetRefCache(tabID string) *refCache {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.snapshots[tabID]
}

// SetRefCache stores the snapshot ref cache for a tab.
func (b *Bridge) SetRefCache(tabID string, cache *refCache) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snapshots[tabID] = cache
}

// DeleteRefCache removes the snapshot ref cache for a tab.
func (b *Bridge) DeleteRefCache(tabID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.snapshots, tabID)
}

// CreateTab opens a new tab, navigates to url, and returns its ID and context.
func (b *Bridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	if b.browserCtx == nil {
		return "", nil, nil, fmt.Errorf("no browser context available")
	}
	ctx, cancel := chromedp.NewContext(b.browserCtx)

	// Inject stealth script before navigation so it applies to the first page load
	b.injectStealth(ctx)

	// Apply global resource blocking on new tabs
	if blockMedia {
		_ = setResourceBlocking(ctx, mediaBlockPatterns)
	} else if blockImages {
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
	b.mu.Lock()
	b.tabs[newTargetID] = &TabEntry{ctx: ctx, cancel: cancel}
	b.mu.Unlock()

	return newTargetID, ctx, cancel, nil
}

// CloseTab closes a tab by ID and cleans up caches.
func (b *Bridge) CloseTab(tabID string) error {
	b.mu.Lock()
	entry, tracked := b.tabs[tabID]
	b.mu.Unlock()

	// Cancel the tab's chromedp context — this is the idiomatic way to close
	// a tab in chromedp. It detaches from the target and lets Chrome clean up.
	if tracked && entry.cancel != nil {
		entry.cancel()
	}

	// Also send target.CloseTarget via the browser context for a clean CDP close.
	closeCtx, closeCancel := context.WithTimeout(b.browserCtx, 5*time.Second)
	defer closeCancel()

	// Use the CDP executor directly (not chromedp.Run) to avoid context issues.
	if err := target.CloseTarget(target.ID(tabID)).Do(cdp.WithExecutor(closeCtx, chromedp.FromContext(closeCtx).Browser)); err != nil {
		// Target may already be gone after cancel — that's fine.
		if !tracked {
			return nil
		}
		slog.Debug("close target CDP", "tabId", tabID, "err", err)
	}

	// Clean up local state.
	b.mu.Lock()
	delete(b.tabs, tabID)
	delete(b.snapshots, tabID)
	b.mu.Unlock()

	return nil
}

// ListTargets returns all open page targets from Chrome.
func (b *Bridge) ListTargets() ([]*target.Info, error) {
	if b.browserCtx == nil {
		return nil, fmt.Errorf("no browser connection")
	}
	var targets []*target.Info
	if err := chromedp.Run(b.browserCtx,
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
