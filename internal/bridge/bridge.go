package bridge

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

type TabEntry struct {
	Ctx                   context.Context
	Cancel                context.CancelFunc
	Accessed              bool
	CDPID                 string    // raw CDP target ID
	CreatedAt             time.Time // when the tab was first created/registered
	LastUsed              time.Time // last time the tab was accessed via TabContext
	Policy                TabPolicyState
	Watching              bool
	ConsoleCaptureEnabled bool
}

type RefCache struct {
	Refs  map[string]int64
	Nodes []A11yNode
}

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
		LogStore:            logStore,
		stealthLaunchMode:   stealth.LaunchModeUninitialized,
	}
	b.ensureStealthBundle()
	// Only initialize TabManager if browserCtx is provided (not lazy-init case)
	if cfg != nil && browserCtx != nil {
		b.TabManager = NewTabManager(browserCtx, cfg, idMgr, logStore, b.tabSetup)
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
		if !b.quietStealthObservers() {
			b.StartBrowserGuards()
		}
	}
	b.Locks = NewLockManager()
	b.Dialogs = NewDialogManager()
	b.InitActionRegistry()
	return b
}

func (b *Bridge) quietStealthObservers() bool {
	return b != nil && b.Config != nil && stealth.NormalizeLevel(b.Config.StealthLevel) == stealth.LevelFull
}

func (b *Bridge) RestartStatus() (bool, time.Duration) {
	if b == nil {
		return false, 0
	}
	b.initMu.Lock()
	defer b.initMu.Unlock()
	if !b.draining {
		return false, 0
	}
	remaining := time.Until(b.drainUntil)
	if remaining < 0 {
		remaining = 0
	}
	return true, remaining
}

func (b *Bridge) injectStealth(ctx context.Context) {
	if b.StealthBundle == nil || b.StealthBundle.Script == "" {
		return
	}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(b.StealthBundle.Script).Do(ctx)
			return err
		}),
	); err != nil {
		slog.Warn("stealth injection failed", "err", err)
	}
}

func (b *Bridge) applyTargetStealth(ctx context.Context) {
	if b == nil || b.Config == nil {
		return
	}

	ua := ""
	if b.StealthBundle != nil {
		ua = b.StealthBundle.LaunchUserAgent()
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return stealth.ApplyTargetEmulation(ctx, b.Config, ua)
	})); err != nil {
		slog.Warn("stealth target emulation failed", "err", err)
	}
}

func (b *Bridge) tabSetup(ctx context.Context) {
	b.applyTargetStealth(ctx)
	b.installWorkerStealthParity(ctx)
	b.injectStealth(ctx)
	if b.Config.NoAnimations {
		if err := b.InjectNoAnimations(ctx); err != nil {
			slog.Warn("no-animations injection failed", "err", err)
		}
	}
}

// StartNetworkCapture enables network monitoring for a specific tab.
// This is called lazily when network data is first requested for a tab.
func (b *Bridge) StartNetworkCapture(tabCtx context.Context, tabID string) error {
	if b.netMonitor == nil {
		return fmt.Errorf("network monitor not initialized")
	}
	return b.netMonitor.StartCapture(tabCtx, tabID)
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

func (b *Bridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	b.initMu.Lock()
	defer b.initMu.Unlock()

	if b.draining {
		return ErrBrowserDraining
	}

	if b.initialized && b.BrowserCtx != nil {
		// Check if browser context is still alive
		if b.BrowserCtx.Err() == nil {
			return nil
		}
		// Chrome died — reset state for re-initialization
		slog.Warn("chrome context cancelled, re-initializing")
		b.initialized = false
		b.BrowserCtx = nil
		b.BrowserCancel = nil
		b.AllocCtx = nil
		b.AllocCancel = nil
		b.TabManager = nil
	}

	if b.BrowserCtx != nil {
		if b.BrowserCtx.Err() == nil {
			return nil
		}
		b.BrowserCtx = nil
		b.BrowserCancel = nil
	}

	slog.Debug("ensure chrome called", "headless", cfg.Headless, "profile", cfg.ProfileDir)

	// Initialize Chrome if not already done
	if err := AcquireProfileLock(cfg.ProfileDir); err != nil {
		if cfg.Headless {
			// If we are in headless mode, we are more flexible.
			// Instead of failing, we can use a unique temporary profile dir.
			uniqueDir, tmpErr := os.MkdirTemp("", "pinchtab-profile-*")
			if tmpErr == nil {
				slog.Warn("profile in use; using unique temporary profile for headless instance",
					"requested", cfg.ProfileDir, "using", uniqueDir, "reason", err.Error())
				cfg.ProfileDir = uniqueDir
				b.tempProfileDir = uniqueDir
				// Re-acquire lock for the new temp dir (should always succeed)
				_ = AcquireProfileLock(cfg.ProfileDir)
			} else {
				slog.Error("cannot acquire profile lock and failed to create temp dir", "profile", cfg.ProfileDir, "err", err.Error(), "tmpErr", tmpErr.Error())
				return fmt.Errorf("profile lock: %w (temp dir failed: %v)", err, tmpErr)
			}
		} else {
			slog.Error("cannot acquire profile lock; another pinchtab may be active", "profile", cfg.ProfileDir, "err", err.Error())
			return fmt.Errorf("profile lock: %w", err)
		}
	}

	slog.Info("starting chrome with confirmed profile", "headless", cfg.Headless, "profile", cfg.ProfileDir)
	b.ensureStealthBundle()
	allocCtx, allocCancel, browserCtx, browserCancel, launchMode, err := InitChrome(cfg, b.StealthBundle)
	if err != nil {
		return fmt.Errorf("failed to initialize chrome: %w", err)
	}

	b.AllocCtx = allocCtx
	b.AllocCancel = allocCancel
	b.BrowserCtx = browserCtx
	b.BrowserCancel = browserCancel
	b.initialized = true
	b.stealthLaunchMode = launchMode

	// Initialize TabManager now that browser is ready
	if b.Config != nil && b.TabManager == nil {
		if b.IdMgr == nil {
			b.IdMgr = ids.NewManager()
		}
		if b.LogStore == nil {
			b.LogStore = NewConsoleLogStore(1000)
		}
		b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.LogStore, b.tabSetup)
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
		if !b.quietStealthObservers() {
			b.StartBrowserGuards()
		}
	}

	// Ensure action registry is populated (idempotent)
	if b.Actions == nil {
		b.InitActionRegistry()
	}

	// Restore tabs from previous session (if any saved state exists)
	if b.tempProfileDir == "" {
		b.RestoreState()
	}

	// Start crash monitoring
	if !b.quietStealthObservers() {
		b.MonitorCrashes(nil)
	}

	return nil
}

// Cleanup releases browser resources and removes temporary profile directories.
// Must be called on shutdown to prevent Chrome process and disk leaks.
func (b *Bridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	if cfg == nil {
		cfg = b.Config
	}
	if cfg == nil {
		return fmt.Errorf("runtime config is required")
	}

	const drainWindow = 2 * time.Second

	b.initMu.Lock()
	b.draining = true
	b.drainUntil = time.Now().Add(drainWindow)
	b.initMu.Unlock()

	slog.Info("browser soft restart: draining requests before restart", "drain_window", drainWindow)
	time.Sleep(drainWindow)

	b.initMu.Lock()

	if b.BrowserCancel != nil {
		b.BrowserCancel()
		slog.Info("browser soft restart: cancelled browser context")
	}
	if b.AllocCancel != nil {
		b.AllocCancel()
		slog.Info("browser soft restart: cancelled allocator context")
	}

	profileDir := ""
	if b.tempProfileDir != "" {
		profileDir = b.tempProfileDir
	} else {
		profileDir = cfg.ProfileDir
	}
	if profileDir != "" {
		time.Sleep(200 * time.Millisecond)
		killed := killChromeByProfileDir(profileDir)
		if killed > 0 {
			slog.Info("browser soft restart: killed surviving chrome processes", "count", killed, "profileDir", profileDir)
		}
		ClearChromeSessions(profileDir)
	}
	b.ClearSavedState()

	if b.tempProfileDir != "" {
		if err := os.RemoveAll(b.tempProfileDir); err != nil {
			slog.Warn("failed to remove temp profile dir during restart", "path", b.tempProfileDir, "err", err)
		} else {
			slog.Info("removed temp profile dir during restart", "path", b.tempProfileDir)
		}
		b.tempProfileDir = ""
	}

	b.initialized = false
	b.BrowserCtx = nil
	b.BrowserCancel = nil
	b.AllocCtx = nil
	b.AllocCancel = nil
	b.TabManager = nil
	b.stealthLaunchMode = stealth.LaunchModeUninitialized

	b.LogStore = NewConsoleLogStore(1000)
	b.netMonitor = NewNetworkMonitor(DefaultNetworkBufferSize)
	if cfg.NetworkBufferSize > 0 {
		b.netMonitor = NewNetworkMonitor(cfg.NetworkBufferSize)
	}
	b.fingerprintMu.Lock()
	b.fingerprintOverlays = make(map[string]bool)
	b.fingerprintMu.Unlock()
	b.workerStealthTargets = sync.Map{}
	b.Dialogs = NewDialogManager()
	b.Locks = NewLockManager()
	b.Config = cfg

	b.StealthBundle = nil
	b.Actions = nil
	b.InitActionRegistry()

	b.draining = false
	b.drainUntil = time.Time{}
	b.initMu.Unlock()

	if err := b.EnsureChrome(cfg); err != nil {
		return err
	}
	b.CleanupSavedStateBackup()
	return nil
}

func (b *Bridge) Cleanup() {
	// Persist open tabs so next startup can restore them
	if b.TabManager != nil && b.tempProfileDir == "" {
		b.SaveState()
	}

	// Mark a clean exit so Chrome doesn't show a crash recovery bar
	if b.Config != nil && b.tempProfileDir == "" {
		MarkCleanExit(b.Config.ProfileDir)
	}

	// Cancel chromedp contexts (kills main Chrome process)
	if b.BrowserCancel != nil {
		b.BrowserCancel()
		slog.Debug("chrome browser context cancelled")
	}
	if b.AllocCancel != nil {
		b.AllocCancel()
		slog.Debug("chrome allocator context cancelled")
	}

	// Chrome spawns helpers (GPU, renderer) in their own process groups.
	// Context cancellation only kills the main process. Kill survivors
	// by scanning for processes using our profile directory.
	profileDir := ""
	if b.tempProfileDir != "" {
		profileDir = b.tempProfileDir
	} else if b.Config != nil {
		profileDir = b.Config.ProfileDir
	}
	if profileDir != "" {
		// Brief wait for context cancel to propagate
		time.Sleep(200 * time.Millisecond)
		killed := killChromeByProfileDir(profileDir)
		if killed > 0 {
			slog.Info("cleanup: killed surviving chrome processes", "count", killed, "profileDir", profileDir)
		}
	}

	if b.tempProfileDir != "" {
		if err := os.RemoveAll(b.tempProfileDir); err != nil {
			slog.Warn("failed to remove temp profile dir", "path", b.tempProfileDir, "err", err)
		} else {
			slog.Info("removed temp profile dir", "path", b.tempProfileDir)
		}
		b.tempProfileDir = ""
	}
}

func (b *Bridge) SetBrowserContexts(allocCtx context.Context, allocCancel context.CancelFunc, browserCtx context.Context, browserCancel context.CancelFunc) {
	b.initMu.Lock()
	defer b.initMu.Unlock()

	b.AllocCtx = allocCtx
	b.AllocCancel = allocCancel
	b.BrowserCtx = browserCtx
	b.BrowserCancel = browserCancel
	b.initialized = true
	b.stealthLaunchMode = stealth.LaunchModeAttached

	// Now initialize TabManager with the browser context
	if b.Config != nil && b.TabManager == nil {
		if b.IdMgr == nil {
			b.IdMgr = ids.NewManager()
		}
		if b.LogStore == nil {
			b.LogStore = NewConsoleLogStore(1000)
		}
		b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.LogStore, b.tabSetup)
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
	}
}

func (b *Bridge) ensureStealthBundle() {
	if b.StealthBundle != nil || b.Config == nil {
		return
	}
	b.StealthBundle = stealth.NewBundle(b.Config, cryptoRandSeed())
}

func (b *Bridge) StealthStatus() *stealth.Status {
	b.ensureStealthBundle()
	return stealth.StatusFromBundle(b.StealthBundle, b.Config, b.stealthLaunchMode)
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

func (b *Bridge) ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error) {
	fn, ok := b.Actions[kind]
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", kind)
	}
	guardEnabled := b.Config == nil || b.Config.EnableActionGuards
	checkNav := guardEnabled && shouldCheckUnexpectedNavigation(req)
	urlReader := b.URLReader
	if urlReader == nil {
		urlReader = defaultActionURLReader
	}
	var beforeURL string
	if checkNav {
		if u, err := urlReader(ctx); err == nil {
			beforeURL = u
		}
	}

	res, err := fn(ctx, req)
	if err != nil {
		return nil, classifyActionError(err)
	}

	if checkNav && beforeURL != "" {
		afterURL, uErr := urlReader(ctx)
		if uErr == nil {
			if navErr := checkUnexpectedNavigation(beforeURL, afterURL); navErr != nil {
				return nil, navErr
			}
		}
	}

	return res, nil
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

func (b *Bridge) AvailableActions() []string {
	keys := make([]string, 0, len(b.Actions))
	for k := range b.Actions {
		keys = append(keys, k)
	}
	return keys
}

// GetDialogManager returns the bridge's dialog manager.
func (b *Bridge) GetDialogManager() *DialogManager {
	return b.Dialogs
}

// ActionFunc is the type for action handlers.
type ActionFunc func(ctx context.Context, req ActionRequest) (map[string]any, error)

// ActionRequest defines the parameters for a browser action.
//
// Element targeting uses a unified selector string that supports multiple
// strategies via prefix detection (see the selector package):
//
//	"e5"              → ref from snapshot
//	"css:#login"      → CSS selector (explicit)
//	"#login"          → CSS selector (auto-detected)
//	"xpath://div"     → XPath expression
//	"text:Submit"     → text content match
//	"find:login btn"  → semantic / natural-language query
//
// For backward compatibility, the legacy Ref and Selector (CSS) fields
// are still accepted. Call NormalizeSelector() to merge them into the
// unified Selector field.
type ActionRequest struct {
	TabID    string `json:"tabId"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref,omitempty"`
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	NodeID   int64  `json:"nodeId"`

	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	HasXY bool    `json:"hasXY,omitempty"`

	ScrollX int `json:"scrollX"`
	ScrollY int `json:"scrollY"`
	DragX   int `json:"dragX"`
	DragY   int `json:"dragY"`

	WaitNav bool   `json:"waitNav"`
	Fast    bool   `json:"fast"`
	Owner   string `json:"owner"`
}

// NormalizeSelector merges legacy Ref and Selector (CSS) fields into the
// unified Selector field. After calling this, only Selector needs to be
// inspected for element targeting. The method is idempotent.
//
// Priority: Ref > Selector (if both are set, Ref wins).
func (r *ActionRequest) NormalizeSelector() {
	if r.Ref != "" && r.Selector == "" {
		// Legacy ref field → unified selector
		r.Selector = r.Ref
	}
	// If Selector is already set (either from JSON or from Ref promotion),
	// leave it as-is — Parse() will auto-detect the kind.
}

func cryptoRandSeed() int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000000))
	if err != nil {
		return 42
	}
	return n.Int64()
}
