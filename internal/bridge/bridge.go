package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

type Bridge struct {
	AllocCtx      context.Context
	AllocCancel   context.CancelFunc
	BrowserCtx    context.Context
	BrowserCancel context.CancelFunc
	Config        *config.RuntimeConfig
	URLReader     URLReader
	IdMgr         *ids.Manager
	*TabManager
	StealthBundle *stealth.Bundle
	Actions       map[string]ActionFunc
	Locks         *LockManager
	Dialogs       *DialogManager
	LogStore      *ConsoleLogStore

	// Network monitoring
	netMonitor *NetworkMonitor

	// Network route interception (Fetch domain). Lazy: enables CDP fetch
	// only when at least one rule is active for a tab.
	routeMgr *RouteManager

	fingerprintMu        sync.RWMutex
	fingerprintOverlays  map[string]bool
	workerStealthTargets sync.Map
	handoffMu            sync.RWMutex
	handoffs             map[string]TabHandoffState
	pointerMu            sync.RWMutex
	pointerByTab         map[string]pointerState

	// Runtime is the provider-specific RuntimeInstance for this browser
	// session. Initialized during EnsureChrome. Nil before launch.
	Runtime browsers.RuntimeInstance

	// Lazy initialization / restart coordination
	initMu      sync.Mutex
	initialized bool
	draining    bool
	drainUntil  time.Time

	// Temp profile cleanup: directories created as fallback when profile lock fails.
	// These are removed on Cleanup() to prevent Chrome process/disk leaks.
	tempProfileDir string

	stealthLaunchMode stealth.LaunchMode

	// captureFunc overrides CaptureScreenshot in polling screencast for testing.
	captureFunc func(ctx context.Context, format string, quality int) ([]byte, error)
}

func New(allocCtx, browserCtx context.Context, cfg *config.RuntimeConfig) *Bridge {
	idMgr := ids.NewManager()
	netBufSize := DefaultNetworkBufferSize
	if cfg != nil && cfg.NetworkBufferSize > 0 {
		netBufSize = cfg.NetworkBufferSize
	}
	logStore := NewConsoleLogStore(1000)
	b := &Bridge{
		AllocCtx:            allocCtx,
		BrowserCtx:          browserCtx,
		Config:              cfg,
		IdMgr:               idMgr,
		netMonitor:          NewNetworkMonitor(netBufSize),
		fingerprintOverlays: make(map[string]bool),
		handoffs:            make(map[string]TabHandoffState),
		pointerByTab:        make(map[string]pointerState),
		LogStore:            logStore,
		stealthLaunchMode:   stealth.LaunchModeUninitialized,
	}
	if cfg != nil {
		b.netMonitor.ConfigureBodyRetention(cfg.RetainNetworkBodies, cfg.RetainNetworkBodyMaxBytes)
	}
	b.routeMgr = NewRouteManager(func() []string {
		if b.Config == nil {
			return nil
		}
		return b.Config.AllowedDomains
	})
	b.ensureStealthBundle()
	b.Dialogs = NewDialogManager()
	if cfg != nil && browserCtx != nil {
		b.TabManager = NewTabManager(browserCtx, cfg, idMgr, logStore, b.tabSetup)
		b.SetOnAfterClose(func() { go b.SaveState() })
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
		b.SetRouteManager(b.routeMgr)
		if !b.quietStealthObservers() {
			b.StartBrowserGuards()
		}
	}
	b.Locks = NewLockManager()
	b.InitActionRegistry()
	return b
}

// StartNetworkCapture enables network monitoring for a specific tab.
// This is called lazily when network data is first requested for a tab.
func (b *Bridge) StartNetworkCapture(tabCtx context.Context, tabID string) error {
	if b.netMonitor == nil {
		return fmt.Errorf("network monitor not initialized")
	}
	return b.netMonitor.StartCapture(tabCtx, tabID)
}

func (b *Bridge) tabManager() (*TabManager, error) {
	if b == nil || b.TabManager == nil {
		return nil, fmt.Errorf("tab manager not initialized")
	}
	return b.TabManager, nil
}

func (b *Bridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	tm, err := b.tabManager()
	if err != nil {
		return "", nil, nil, err
	}
	tabID, ctx, cancel, err := tm.CreateTab(url)
	if err == nil {
		go b.SaveState()
	}
	return tabID, ctx, cancel, err
}

func (b *Bridge) TabContext(tabID string) (*TabHandle, string, error) {
	tm, err := b.tabManager()
	if err != nil {
		return nil, "", err
	}
	ctx, resolved, err := tm.TabContext(tabID)
	if err != nil {
		return nil, "", err
	}
	return NewTabHandle(ctx), resolved, nil
}

func (b *Bridge) ListTargets() ([]TabTarget, error) {
	tm, err := b.tabManager()
	if err != nil {
		return nil, err
	}
	infos, err := tm.ListTargets()
	if err != nil {
		return nil, err
	}
	targets := make([]TabTarget, len(infos))
	for i, t := range infos {
		targets[i] = TabTarget{
			TargetID: string(t.TargetID),
			URL:      t.URL,
			Title:    t.Title,
			Type:     string(t.Type),
		}
	}
	return targets, nil
}

func (b *Bridge) CloseTab(tabID string) error {
	tm, err := b.tabManager()
	if err != nil {
		return err
	}
	// SaveState is triggered via TabManager.onAfterClose, set during construction.
	return tm.CloseTab(tabID)
}

func (b *Bridge) FocusTab(tabID string) error {
	tm, err := b.tabManager()
	if err != nil {
		return err
	}
	return tm.FocusTab(tabID)
}

func (b *Bridge) ScheduleAutoClose(tabID string) {
	tm, err := b.tabManager()
	if err != nil {
		return
	}
	tm.ScheduleAutoClose(tabID)
}

func (b *Bridge) CancelAutoClose(tabID string) {
	tm, err := b.tabManager()
	if err != nil {
		return
	}
	tm.CancelAutoClose(tabID)
}

func (b *Bridge) Lock(tabID, owner string, ttl time.Duration) error {
	return b.Locks.TryLock(tabID, owner, ttl)
}

func (b *Bridge) Unlock(tabID, owner string) error {
	return b.Locks.Unlock(tabID, owner)
}

func (b *Bridge) TabLockInfo(tabID string) *LockInfo {
	return b.Locks.Get(tabID)
}

// GetConsoleLogs returns console logs for a tab.
func (b *Bridge) GetConsoleLogs(tabID string, limit int) []LogEntry {
	if b.LogStore == nil {
		return nil
	}
	if b.TabManager != nil {
		b.EnsureConsoleCapture(tabID)
	}
	return b.LogStore.GetConsoleLogs(tabID, limit)
}

// ClearConsoleLogs clears console logs for a tab.
func (b *Bridge) ClearConsoleLogs(tabID string) {
	if b.LogStore != nil {
		b.LogStore.ClearConsoleLogs(tabID)
	}
}

// GetErrorLogs returns error logs for a tab.
func (b *Bridge) GetErrorLogs(tabID string, limit int) []ErrorEntry {
	if b.LogStore == nil {
		return nil
	}
	if b.TabManager != nil {
		b.EnsureConsoleCapture(tabID)
	}
	return b.LogStore.GetErrorLogs(tabID, limit)
}

// ClearErrorLogs clears error logs for a tab.
func (b *Bridge) ClearErrorLogs(tabID string) {
	if b.LogStore != nil {
		b.LogStore.ClearErrorLogs(tabID)
	}
}

func (b *Bridge) SetFingerprintRotateActive(tabID string, active bool) {
	if tabID == "" {
		return
	}
	b.fingerprintMu.Lock()
	defer b.fingerprintMu.Unlock()
	b.fingerprintOverlays[tabID] = active
}

func (b *Bridge) FingerprintRotateActive(tabID string) bool {
	if tabID == "" {
		return false
	}
	b.fingerprintMu.RLock()
	defer b.fingerprintMu.RUnlock()
	return b.fingerprintOverlays[tabID]
}

func (b *Bridge) BrowserContext() context.Context {
	return b.BrowserCtx
}

// Navigate navigates the given context to a URL, respecting redirect limits,
// then reads back the final URL and page title.
func (b *Bridge) Navigate(ctx context.Context, url string, params NavigateParams) (*NavigateResult, error) {
	if err := NavigatePageWithRedirectLimit(ctx, url, params.MaxRedirects); err != nil {
		return nil, err
	}

	var finalURL string
	if err := chromedp.Run(ctx, chromedp.Location(&finalURL)); err != nil {
		return nil, fmt.Errorf("read final URL: %w", err)
	}

	title, _ := WaitForTitle(ctx, 2*time.Second)

	return &NavigateResult{
		URL:   finalURL,
		Title: title,
	}, nil
}

// Snapshot fetches the accessibility tree for the given tab context, builds
// the filtered snapshot, enriches nodes with DOM metadata, and returns the
// complete result. The ctx must already be a tab-scoped chromedp context;
// tabID is for bookkeeping only. ContentParams is accepted but ignored by
// the base Bridge (IDPI scanning is the adapter's responsibility).
func (b *Bridge) Snapshot(ctx context.Context, tabID string, filter string, params ContentParams) (*SnapshotResult, error) {
	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch AX tree: %w", err)
	}

	maxDepth := params.MaxDepth
	if maxDepth == 0 {
		maxDepth = -1
	}
	nodes, refs := BuildSnapshot(rawNodes, filter, maxDepth)
	_ = EnrichA11yNodesWithDOMMetadata(ctx, nodes)
	targets := RefTargetsFromNodes(nodes)

	var url string
	if err := chromedp.Run(ctx, chromedp.Location(&url)); err != nil {
		return nil, fmt.Errorf("read URL: %w", err)
	}

	var title string
	if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
		return nil, fmt.Errorf("read title: %w", err)
	}

	return &SnapshotResult{
		Nodes:   nodes,
		Refs:    refs,
		Targets: targets,
		URL:     url,
		Title:   title,
	}, nil
}

// Text extracts the page text from the given tab context using
// document.body.innerText, then reads back the current URL and title.
// The ctx must already be a tab-scoped chromedp context; tabID is for
// bookkeeping only. ContentParams is accepted but ignored by the base
// Bridge (IDPI scanning is the adapter's responsibility).
func (b *Bridge) Text(ctx context.Context, tabID string, params ContentParams) (*TextResult, error) {
	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText`, &text)); err != nil {
		return nil, fmt.Errorf("extract text: %w", err)
	}

	var url string
	if err := chromedp.Run(ctx, chromedp.Location(&url)); err != nil {
		return nil, fmt.Errorf("read URL: %w", err)
	}

	var title string
	if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
		return nil, fmt.Errorf("read title: %w", err)
	}

	return &TextResult{
		Text:  text,
		URL:   url,
		Title: title,
	}, nil
}

// Execute delegates to TabManager.Execute for safe parallel tab execution.
// If TabManager is not initialized, the task runs directly.
func (b *Bridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	if b.TabManager != nil {
		return b.TabManager.Execute(ctx, tabID, task)
	}
	return task(ctx)
}

// NetworkMonitor returns the bridge's network monitor instance.
func (b *Bridge) NetworkMonitor() *NetworkMonitor {
	return b.netMonitor
}

// AddRouteRule installs (or replaces by Pattern) an interception rule for the
// given tab. The first call for a tab enables CDP Fetch interception.
func (b *Bridge) AddRouteRule(tabID string, rule RouteRule) error {
	if b.routeMgr == nil {
		return fmt.Errorf("route manager not initialized")
	}
	tabHandle, resolvedID, err := b.TabContext(tabID)
	if err != nil {
		return err
	}
	return b.routeMgr.AddRule(tabHandle, resolvedID, rule)
}

// RemoveRouteRule removes a rule by Pattern, or all rules when pattern is empty.
// Returns the count of rules removed.
func (b *Bridge) RemoveRouteRule(tabID, pattern string) (int, error) {
	if b.routeMgr == nil {
		return 0, fmt.Errorf("route manager not initialized")
	}
	tabHandle, resolvedID, err := b.TabContext(tabID)
	if err != nil {
		return 0, err
	}
	return b.routeMgr.Remove(tabHandle, resolvedID, pattern)
}

// ListRouteRules returns the current interception rules for a tab.
func (b *Bridge) ListRouteRules(tabID string) ([]RouteRule, error) {
	if b.routeMgr == nil {
		return nil, fmt.Errorf("route manager not initialized")
	}
	_, resolvedID, err := b.TabContext(tabID)
	if err != nil {
		return nil, err
	}
	return b.routeMgr.List(resolvedID), nil
}

// GetDialogManager returns the bridge's dialog manager.
func (b *Bridge) GetDialogManager() *DialogManager {
	return b.Dialogs
}
