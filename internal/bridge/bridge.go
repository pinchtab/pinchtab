package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/target"
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

	fingerprintMu        sync.RWMutex
	fingerprintOverlays  map[string]bool
	workerStealthTargets sync.Map
	handoffMu            sync.RWMutex
	handoffs             map[string]TabHandoffState
	pointerMu            sync.RWMutex
	pointerByTab         map[string]pointerState

	// Lazy initialization / restart coordination
	initMu      sync.Mutex
	initialized bool
	draining    bool
	drainUntil  time.Time

	// Temp profile cleanup: directories created as fallback when profile lock fails.
	// These are removed on Cleanup() to prevent Chrome process/disk leaks.
	tempProfileDir string

	stealthLaunchMode stealth.LaunchMode
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
	b.ensureStealthBundle()
	b.Dialogs = NewDialogManager()
	if cfg != nil && browserCtx != nil {
		b.TabManager = NewTabManager(browserCtx, cfg, idMgr, logStore, b.tabSetup)
		b.SetOnAfterClose(func() { go b.SaveState() })
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
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

func (b *Bridge) TabContext(tabID string) (context.Context, string, error) {
	tm, err := b.tabManager()
	if err != nil {
		return nil, "", err
	}
	return tm.TabContext(tabID)
}

func (b *Bridge) ListTargets() ([]*target.Info, error) {
	tm, err := b.tabManager()
	if err != nil {
		return nil, err
	}
	return tm.ListTargets()
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

// GetDialogManager returns the bridge's dialog manager.
func (b *Bridge) GetDialogManager() *DialogManager {
	return b.Dialogs
}
