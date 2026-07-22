package bridge

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
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

	netMonitor *NetworkMonitor

	// Network route interception (Fetch domain). Lazy: enables CDP fetch
	// only when at least one rule is active for a tab.
	routeMgr *RouteManager

	// fetchPauseMu guards fetchPauseFlags: per-tab suppression of the
	// proxy-auth listener's blanket ContinueRequest while another Fetch
	// user (route rules, credentials handler) owns request-pause dispatch.
	fetchPauseMu    sync.Mutex
	fetchPauseFlags map[string]*atomic.Bool

	// tabRemovedHooksMu guards externalTabRemovedHooks: tab-removal cleanups
	// registered from outside the bridge (e.g. the credentials handler). They
	// are stored here, not just on the TabManager, so wireTabManager can
	// re-apply them when a launch/reinit/remote-CDP path swaps the TabManager.
	tabRemovedHooksMu       sync.Mutex
	externalTabRemovedHooks []func(string)

	fingerprintMu        sync.RWMutex
	fingerprintOverlays  map[string]bool
	workerStealthTargets sync.Map
	attachIndicatorMu    sync.Mutex
	attachIndicators     map[string]attachIndicatorState
	handoffMu            sync.RWMutex
	handoffs             map[string]TabHandoffState
	pointerMu            sync.RWMutex
	pointerByTab         map[string]pointerState

	// Initialized during EnsureBrowser. Nil before launch.
	Runtime browsers.RuntimeInstance

	initMu      sync.Mutex
	initialized bool
	draining    bool
	drainUntil  time.Time

	// Temp profile cleanup: directories created as fallback when profile lock fails.
	// These are removed on Cleanup() to prevent Chrome process/disk leaks.
	tempProfileDir string

	stealthLaunchMode stealth.LaunchMode

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
	b.routeMgr.SetFetchAuthCoordination(
		func() bool { return b.Config != nil && bridgeruntime.ProxyAuthEnabled(b.Config.Proxy) },
		b.SetFetchPauseSuppressed,
	)
	b.ensureStealthBundle()
	b.Dialogs = NewDialogManager()
	if cfg != nil && browserCtx != nil {
		b.wireTabManager(browserCtx)
		if !b.quietStealthObservers() {
			b.StartBrowserGuards()
		}
	}
	b.Locks = NewLockManager()
	b.InitActionRegistry()
	return b
}

func (b *Bridge) fetchPauseSuppression(tabID string) *atomic.Bool {
	if tabID == "" {
		return nil
	}
	b.fetchPauseMu.Lock()
	defer b.fetchPauseMu.Unlock()
	if b.fetchPauseFlags == nil {
		b.fetchPauseFlags = map[string]*atomic.Bool{}
	}
	flag, ok := b.fetchPauseFlags[tabID]
	if !ok {
		flag = &atomic.Bool{}
		b.fetchPauseFlags[tabID] = flag
	}
	return flag
}

func (b *Bridge) SetFetchPauseSuppressed(tabID string, v bool) {
	if b == nil || tabID == "" {
		return
	}
	b.fetchPauseSuppression(tabID).Store(v)
}

func (b *Bridge) dropFetchPauseSuppression(tabID string) {
	b.fetchPauseMu.Lock()
	defer b.fetchPauseMu.Unlock()
	delete(b.fetchPauseFlags, tabID)
}

// AddTabRemovedHook registers an external per-tab cleanup that must survive a
// TabManager swap. It records the hook on the bridge and applies it to the
// current TabManager; wireTabManager re-applies all recorded hooks after a
// launch/reinit/remote-CDP path replaces the TabManager. Shadows the embedded
// TabManager.AddTabRemovedHook so external callers always go through here.
func (b *Bridge) AddTabRemovedHook(fn func(tabID string)) {
	if fn == nil {
		return
	}
	b.tabRemovedHooksMu.Lock()
	b.externalTabRemovedHooks = append(b.externalTabRemovedHooks, fn)
	b.tabRemovedHooksMu.Unlock()
	if b.TabManager != nil {
		b.TabManager.AddTabRemovedHook(fn)
	}
}

// reinitWiringOpts parameterizes the differences between the launch, restart,
// and remote-CDP wiring paths while keeping their shared guard+wire+registry
// core in one place (see reinitWiring).
type reinitWiringOpts struct {
	// startBrowserGuards starts the popup/crash guards after wiring (skipped in
	// full stealth and on paths that never ran them, e.g. SetBrowserContexts and
	// remote-CDP).
	startBrowserGuards bool
	// initActionRegistry seeds the action registry when it is nil (skipped by
	// SetBrowserContexts, which leaves registry init to a later path).
	initActionRegistry bool
}

// reinitWiring owns the IdMgr/LogStore nil-guards, the TabManager wiring (via
// wireTabManager, gated on TabManager being nil), and the optional
// StartBrowserGuards / InitActionRegistry steps that every launch/reinit/
// remote-CDP path repeats. Centralizing the sequence here keeps the paths from
// drifting; per-path differences are expressed through opts.
func (b *Bridge) reinitWiring(browserCtx context.Context, opts reinitWiringOpts) {
	if b.IdMgr == nil {
		b.IdMgr = ids.NewManager()
	}
	if b.LogStore == nil {
		b.LogStore = NewConsoleLogStore(1000)
	}
	if b.TabManager == nil {
		b.wireTabManager(browserCtx)
	}
	if opts.startBrowserGuards && !b.quietStealthObservers() {
		b.StartBrowserGuards()
	}
	if opts.initActionRegistry && b.Actions == nil {
		b.InitActionRegistry()
	}
}

// wireTabManager creates the TabManager for browserCtx and applies the
// post-construction wiring every launch/reinit/remote-CDP path needs, so the
// setup can't drift across paths. Callers gate on their own guards and add any
// path-specific setup (e.g. New's StartBrowserGuards) afterward.
func (b *Bridge) wireTabManager(browserCtx context.Context) {
	b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.LogStore, b.tabSetup)
	b.SetOnAfterClose(func() { go b.SaveState() })
	b.SetDialogManager(b.Dialogs)
	b.SetNetworkMonitor(b.netMonitor)
	b.SetRouteManager(b.routeMgr)
	// Built-in cleanup goes straight onto the fresh TabManager (re-added each
	// wire, so no cross-reinit duplication). External hooks recorded on the
	// bridge are re-applied so they survive the TabManager swap.
	b.TabManager.AddTabRemovedHook(b.dropFetchPauseSuppression)
	b.tabRemovedHooksMu.Lock()
	hooks := make([]func(string), len(b.externalTabRemovedHooks))
	copy(hooks, b.externalTabRemovedHooks)
	b.tabRemovedHooksMu.Unlock()
	for _, fn := range hooks {
		b.TabManager.AddTabRemovedHook(fn)
	}
}

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

func (b *Bridge) CreateTabInBrowserContext(url, browserContextID string) (string, context.Context, context.CancelFunc, error) {
	tm, err := b.tabManager()
	if err != nil {
		return "", nil, nil, err
	}
	tabID, ctx, cancel, err := tm.CreateTabInBrowserContext(url, browserContextID)
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
			TargetID:         string(t.TargetID),
			URL:              t.URL,
			Title:            t.Title,
			Type:             string(t.Type),
			BrowserContextID: string(t.BrowserContextID),
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

func (b *Bridge) GetConsoleLogs(tabID string, limit int) []LogEntry {
	if b.LogStore == nil {
		return nil
	}
	if b.TabManager != nil {
		b.EnsureConsoleCapture(tabID)
	}
	return b.LogStore.GetConsoleLogs(tabID, limit)
}

func (b *Bridge) ClearConsoleLogs(tabID string) {
	if b.LogStore != nil {
		b.LogStore.ClearConsoleLogs(tabID)
	}
}

func (b *Bridge) GetErrorLogs(tabID string, limit int) []ErrorEntry {
	if b.LogStore == nil {
		return nil
	}
	if b.TabManager != nil {
		b.EnsureConsoleCapture(tabID)
	}
	return b.LogStore.GetErrorLogs(tabID, limit)
}

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

func (b *Bridge) Navigate(ctx context.Context, url string, params NavigateParams) (*NavigateResult, error) {
	if params.DispatchOnly {
		if err := DispatchNavigation(ctx, url); err != nil {
			return nil, fmt.Errorf("dispatch navigation: %w", err)
		}
		return &NavigateResult{URL: url}, nil
	}
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

// ctx must already be a tab-scoped chromedp context; tabID is for bookkeeping
// only. ContentParams is accepted but ignored by the base Bridge (IDPI
// scanning is the adapter's responsibility).
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

// ctx must already be a tab-scoped chromedp context; tabID is for bookkeeping
// only. ContentParams is accepted but ignored by the base Bridge (IDPI
// scanning is the adapter's responsibility).
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

// If TabManager is not initialized, the task runs directly.
func (b *Bridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	if b.TabManager != nil {
		return b.TabManager.Execute(ctx, tabID, task)
	}
	return task(ctx)
}

func (b *Bridge) NetworkMonitor() *NetworkMonitor {
	return b.netMonitor
}

// Replaces an existing rule with the same Pattern. The first call for a tab
// enables CDP Fetch interception.
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

// Removes all rules when pattern is empty. Returns the count removed.
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

func (b *Bridge) GetDialogManager() *DialogManager {
	return b.Dialogs
}
