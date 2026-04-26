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
	"github.com/pinchtab/pinchtab/internal/ids"
)

type TabSetupFunc func(ctx context.Context)

type TabManager struct {
	browserCtx   context.Context
	config       *config.RuntimeConfig
	idMgr        *ids.Manager
	tabs         map[string]*TabEntry
	accessed     map[string]bool
	snapshots    map[string]*RefCache
	frameScope   map[string]FrameScope
	onTabSetup   TabSetupFunc
	onAfterClose func() // optional: invoked after any successful CloseTab
	dialogMgr    *DialogManager
	logStore     *ConsoleLogStore
	netMonitor   *NetworkMonitor
	currentTab   string // ID of the most recently used tab
	executor     *TabExecutor
	guardOnce    sync.Once
	mu           sync.RWMutex
}

func NewTabManager(browserCtx context.Context, cfg *config.RuntimeConfig, idMgr *ids.Manager, logStore *ConsoleLogStore, onTabSetup TabSetupFunc) *TabManager {
	if idMgr == nil {
		idMgr = ids.NewManager()
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
		frameScope: make(map[string]FrameScope),
		onTabSetup: onTabSetup,
		logStore:   logStore,
		executor:   NewTabExecutor(maxParallel),
	}
}

// SetDialogManager sets the dialog manager for dialog event tracking on new tabs.
func (tm *TabManager) SetDialogManager(dm *DialogManager) {
	tm.dialogMgr = dm
}

// SetOnAfterClose registers a callback fired whenever a tracked tab is removed
// from the manager — manual /close, eviction, auto-close lifecycle timer, or
// Chrome reporting the target gone (e.g. user closing it in a headed window).
// Used by the parent Bridge to persist session state immediately rather than
// waiting for graceful shutdown.
func (tm *TabManager) SetOnAfterClose(fn func()) {
	tm.onAfterClose = fn
}

// SetNetworkMonitor sets the network monitor for eager network capture on new tabs.
func (tm *TabManager) SetNetworkMonitor(nm *NetworkMonitor) {
	tm.netMonitor = nm
}

// browserExecutorContext returns a context bound to the top-level browser
// executor, suitable for issuing browser-scoped CDP calls (e.g. target.*).
// Shared helper used by tab lifecycle, lookup, popup-guard, and cleanup paths.
func browserExecutorContext(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("no browser context available")
	}
	c := chromedp.FromContext(ctx)
	if c == nil || c.Browser == nil {
		return nil, fmt.Errorf("no browser executor available")
	}
	return cdp.WithExecutor(ctx, c.Browser), nil
}

func (tm *TabManager) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	if tm == nil {
		return "", nil, nil, fmt.Errorf("tab manager not initialized")
	}
	if tm.browserCtx == nil {
		return "", nil, nil, fmt.Errorf("no browser context available")
	}

	if tm.config != nil && tm.config.MaxTabs > 0 {
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
	var targetID target.ID
	createCtx, createCancel := context.WithTimeout(tm.browserCtx, 10*time.Second)
	if err := chromedp.Run(createCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			targetID, err = target.CreateTarget("about:blank").Do(ctx)
			return err
		}),
	); err != nil {
		createCancel()
		return "", nil, nil, fmt.Errorf("create target: %w", err)
	}
	createCancel()

	ctx, cancel := chromedp.NewContext(tm.browserCtx,
		chromedp.WithTargetID(targetID),
	)

	if tm.onTabSetup != nil {
		tm.onTabSetup(ctx)
	}

	var blockPatterns []string

	if tm.config != nil && tm.config.BlockAds {
		blockPatterns = CombineBlockPatterns(blockPatterns, AdBlockPatterns)
	}

	if tm.config != nil && tm.config.BlockMedia {
		blockPatterns = CombineBlockPatterns(blockPatterns, MediaBlockPatterns)
	} else if tm.config != nil && tm.config.BlockImages {
		blockPatterns = CombineBlockPatterns(blockPatterns, ImageBlockPatterns)
	}

	if len(blockPatterns) > 0 {
		_ = SetResourceBlocking(ctx, blockPatterns)
	}

	rawCDPID := string(targetID)
	tabID := tm.idMgr.TabIDFromCDPTarget(rawCDPID)

	// Start network capture before navigation so CDP events are captured.
	if tm.netMonitor != nil {
		if err := tm.netMonitor.StartCapture(ctx, tabID); err != nil {
			slog.Warn("eager network capture failed", "tab", tabID, "err", err)
		}
	}

	if url != "" && url != "about:blank" {
		navCtx, navCancel := context.WithTimeout(ctx, 30*time.Second)
		if err := chromedp.Run(navCtx, chromedp.Navigate(url)); err != nil {
			navCancel()
			cancel()
			if execCtx, execErr := browserExecutorContext(tm.browserCtx); execErr == nil {
				_ = target.CloseTarget(targetID).Do(execCtx)
			}
			return "", nil, nil, fmt.Errorf("navigate: %w", err)
		}
		navCancel()
	}

	now := time.Now()

	if tm.dialogMgr != nil {
		autoAccept := tm.config != nil && tm.config.DialogAutoAccept
		ListenDialogEvents(ctx, tabID, tm.dialogMgr, autoAccept)
		// Page domain must be enabled for Page.javascriptDialogOpening events
		// to be delivered to ListenTarget callbacks.
		if err := EnableDialogEvents(ctx); err != nil {
			slog.Warn("enable dialog events failed", "tabId", tabID, "err", err)
		}
	}

	if tm.shouldEagerlyCaptureConsole() {
		tm.setupConsoleCapture(ctx, rawCDPID)
	}

	tm.mu.Lock()
	tm.tabs[tabID] = &TabEntry{
		Ctx:                   ctx,
		Cancel:                cancel,
		CDPID:                 rawCDPID,
		CreatedAt:             now,
		LastUsed:              now,
		ConsoleCaptureEnabled: tm.shouldEagerlyCaptureConsole(),
	}
	tm.accessed[tabID] = true
	tm.currentTab = tabID
	tm.mu.Unlock()

	tm.startTabPolicyWatcher(tabID, ctx)

	return tabID, ctx, cancel, nil
}

func (tm *TabManager) CloseTab(tabID string) error {
	if tm == nil {
		return fmt.Errorf("tab manager not initialized")
	}
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

	cdpTargetID := tabID
	if tracked && entry.CDPID != "" {
		cdpTargetID = entry.CDPID
	}

	closeCtx, closeCancel := context.WithTimeout(tm.browserCtx, 5*time.Second)
	defer closeCancel()

	execCtx, execErr := browserExecutorContext(closeCtx)
	if execErr != nil {
		if !tracked {
			return fmt.Errorf("tab %s not found", tabID)
		}
		slog.Debug("close target skipped", "tabId", tabID, "cdpId", cdpTargetID, "err", execErr)
		tm.purgeTrackedTabState(tabID, cdpTargetID)
		return nil
	}

	if err := target.CloseTarget(target.ID(cdpTargetID)).Do(execCtx); err != nil {
		if !tracked {
			return fmt.Errorf("tab %s not found", tabID)
		}
		slog.Debug("close target CDP", "tabId", tabID, "cdpId", cdpTargetID, "err", err)
	}
	tm.purgeTrackedTabState(tabID, cdpTargetID)
	return nil
}

// FocusTab activates a tab by ID, bringing it to the foreground and setting it
// as the current tab for subsequent operations.
func (tm *TabManager) FocusTab(tabID string) error {
	if tm == nil {
		return fmt.Errorf("tab manager not initialized")
	}
	ctx, resolvedID, err := tm.TabContext(tabID)
	if err != nil {
		return err
	}

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
