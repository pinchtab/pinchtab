package bridge

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
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/idutil"
)

type TabSetupFunc func(ctx context.Context)

type TabManager struct {
	browserCtx context.Context
	config     *config.RuntimeConfig
	idMgr      *idutil.Manager
	tabs       map[string]*TabEntry
	accessed   map[string]bool
	snapshots  map[string]*RefCache
	onTabSetup TabSetupFunc
	currentTab string // ID of the most recently used tab
	executor   *TabExecutor
	mu         sync.RWMutex
}

func NewTabManager(browserCtx context.Context, cfg *config.RuntimeConfig, idMgr *idutil.Manager, onTabSetup TabSetupFunc) *TabManager {
	if idMgr == nil {
		idMgr = idutil.NewManager()
	}
	maxParallel := 0
	if cfg != nil {
		maxParallel = cfg.MaxParallelTabs
	}
	return &TabManager{
		browserCtx: browserCtx,
		config:     cfg,
		idMgr:      idMgr,
		tabs:       make(map[string]*TabEntry),
		accessed:   make(map[string]bool),
		snapshots:  make(map[string]*RefCache),
		onTabSetup: onTabSetup,
		executor:   NewTabExecutor(maxParallel),
	}
}

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
	// Prefer explicit current tab if still tracked
	if tm.currentTab != "" {
		if _, ok := tm.tabs[tm.currentTab]; ok {
			return tm.currentTab
		}
	}

	// Fallback: pick the tab with the most recent LastUsed
	var best string
	var bestTime time.Time
	for id, entry := range tm.tabs {
		if entry.LastUsed.After(bestTime) {
			best = id
			bestTime = entry.LastUsed
		}
	}
	// If no LastUsed set, fall back to most recent CreatedAt
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
	if tabID == "" {
		// Resolve to current tracked tab
		tm.mu.RLock()
		tabID = tm.selectCurrentTrackedTab()
		tm.mu.RUnlock()

		if tabID == "" {
			// No tracked tabs — try to find one from CDP targets
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
		// Attempt to auto-track the tab if it's open but untracked
		targets, err := tm.ListTargets()
		if err == nil {
			for _, t := range targets {
				raw := string(t.TargetID)
				if tm.idMgr.TabIDFromCDPTarget(raw) == tabID {
					// Initialize context and register it
					ctx, cancel := chromedp.NewContext(tm.browserCtx, chromedp.WithTargetID(target.ID(raw)))
					if tm.onTabSetup != nil {
						tm.onTabSetup(ctx)
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

func (tm *TabManager) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	if tm.browserCtx == nil {
		return "", nil, nil, fmt.Errorf("no browser context available")
	}

	if tm.config.MaxTabs > 0 {
		// Count managed tabs for eviction decisions. Using Chrome's target list
		// would include unmanaged targets (e.g. the initial about:blank tab),
		// causing premature eviction of managed tabs.
		tm.mu.RLock()
		managedCount := len(tm.tabs)
		tm.mu.RUnlock()

		if managedCount >= tm.config.MaxTabs {
			switch tm.config.TabEvictionPolicy {
			case "close_oldest":
				if evictErr := tm.closeOldestTab(); evictErr != nil {
					return "", nil, nil, fmt.Errorf("eviction failed: %w", evictErr)
				}
			case "reject":
				return "", nil, nil, &TabLimitError{Current: managedCount, Max: tm.config.MaxTabs}
			default: // "close_lru" (default)
				if evictErr := tm.closeLRUTab(); evictErr != nil {
					return "", nil, nil, fmt.Errorf("eviction failed: %w", evictErr)
				}
			}
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

	rawCDPID := string(targetID)
	tabID := tm.idMgr.TabIDFromCDPTarget(rawCDPID)
	now := time.Now()

	tm.mu.Lock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx, Cancel: cancel, CDPID: rawCDPID, CreatedAt: now, LastUsed: now}
	tm.accessed[tabID] = true
	tm.currentTab = tabID
	tm.mu.Unlock()

	return tabID, ctx, cancel, nil
}

func (tm *TabManager) CloseTab(tabID string) error {
	// Guard against closing the last tab to prevent Chrome from exiting
	targets, err := tm.ListTargets()
	if err != nil {
		return fmt.Errorf("list targets: %w", err)
	}
	if len(targets) <= 1 {
		return fmt.Errorf("cannot close the last tab — at least one tab must remain")
	}

	tm.mu.Lock()
	entry, tracked := tm.tabs[tabID]
	tm.mu.Unlock()

	if tracked && entry.Cancel != nil {
		entry.Cancel()
	}

	// Resolve to raw CDP target ID for the actual CDP close call
	cdpTargetID := tabID
	if tracked && entry.CDPID != "" {
		cdpTargetID = entry.CDPID
	}

	closeCtx, closeCancel := context.WithTimeout(tm.browserCtx, 5*time.Second)
	defer closeCancel()

	if err := target.CloseTarget(target.ID(cdpTargetID)).Do(cdp.WithExecutor(closeCtx, chromedp.FromContext(closeCtx).Browser)); err != nil {
		if !tracked {
			return fmt.Errorf("tab %s not found", tabID)
		}
		slog.Debug("close target CDP", "tabId", tabID, "cdpId", cdpTargetID, "err", err)
	}

	tm.mu.Lock()
	delete(tm.tabs, tabID)
	delete(tm.snapshots, tabID)
	tm.mu.Unlock()

	// Clean up executor per-tab mutex
	if tm.executor != nil {
		tm.executor.RemoveTab(tabID)
	}

	return nil
}

// FocusTab activates a tab by ID, bringing it to the foreground and setting it
// as the current tab for subsequent operations.
func (tm *TabManager) FocusTab(tabID string) error {
	ctx, resolvedID, err := tm.TabContext(tabID)
	if err != nil {
		return err
	}

	// Bring the tab to front via CDP
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.BringToFront().Do(ctx)
	})); err != nil {
		return fmt.Errorf("bring to front: %w", err)
	}

	tm.mu.Lock()
	tm.currentTab = resolvedID
	if entry, ok := tm.tabs[resolvedID]; ok {
		entry.LastUsed = time.Now()
	}
	tm.mu.Unlock()

	return nil
}

func (tm *TabManager) ListTargets() ([]*target.Info, error) {
	if tm == nil {
		return nil, fmt.Errorf("tab manager not initialized")
	}
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

// ListTargetsWithContext is like ListTargets but uses a custom context
// Useful for short-timeout checks during tab creation
func (tm *TabManager) ListTargetsWithContext(ctx context.Context) ([]*target.Info, error) {
	if tm == nil {
		return nil, fmt.Errorf("tab manager not initialized")
	}
	if tm.browserCtx == nil {
		return nil, fmt.Errorf("no browser connection")
	}
	var targets []*target.Info
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(chromeCtx context.Context) error {
			var err error
			targets, err = target.GetTargets().Do(chromeCtx)
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
	now := time.Now()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx, CreatedAt: now, LastUsed: now}
	tm.currentTab = tabID
}

// RegisterTabWithCancel registers a tab ID with its context and cancel function.
func (tm *TabManager) RegisterTabWithCancel(tabID, rawCDPID string, ctx context.Context, cancel context.CancelFunc) {
	now := time.Now()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx, Cancel: cancel, CDPID: rawCDPID, CreatedAt: now, LastUsed: now}
	tm.currentTab = tabID
}

// Execute runs a task for a tab through the TabExecutor, ensuring per-tab
// sequential execution with cross-tab parallelism bounded by the semaphore.
// If the TabExecutor has not been initialized, the task runs directly.
func (tm *TabManager) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	if tm.executor == nil {
		return task(ctx)
	}
	return tm.executor.Execute(ctx, tabID, task)
}

// Executor returns the underlying TabExecutor (may be nil).
func (tm *TabManager) Executor() *TabExecutor {
	return tm.executor
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

		// Collect stale tab IDs while holding the lock, then clean up
		// executor mutexes outside the lock to avoid blocking TabManager
		// operations if RemoveTab has to wait for an in-flight task.
		var staleIDs []string
		tm.mu.Lock()
		for id, entry := range tm.tabs {
			if !alive[id] {
				if entry.Cancel != nil {
					entry.Cancel()
				}
				delete(tm.tabs, id)
				delete(tm.snapshots, id)
				staleIDs = append(staleIDs, id)
				slog.Info("cleaned stale tab", "id", id)
			}
		}
		tm.mu.Unlock()

		if tm.executor != nil {
			for _, id := range staleIDs {
				tm.executor.RemoveTab(id)
			}
		}
	}
}
