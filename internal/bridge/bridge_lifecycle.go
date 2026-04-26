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

func (b *Bridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	b.initMu.Lock()
	defer b.initMu.Unlock()

	if b.draining {
		return ErrBrowserDraining
	}

	if b.initialized && b.BrowserCtx != nil {
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

	if b.Config != nil && b.TabManager == nil {
		if b.IdMgr == nil {
			b.IdMgr = ids.NewManager()
		}
		if b.LogStore == nil {
			b.LogStore = NewConsoleLogStore(1000)
		}
		b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.LogStore, b.tabSetup)
		b.SetOnAfterClose(func() { go b.SaveState() })
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
		if !b.quietStealthObservers() {
			b.StartBrowserGuards()
		}
	}

	if b.Actions == nil {
		b.InitActionRegistry()
	}

	// Restore tabs from previous session (if any saved state exists). Opt-in
	// via instanceDefaults.tabPolicy.restore=true; otherwise tabs closed
	// before shutdown stay closed.
	if b.tempProfileDir == "" && b.Config != nil && b.Config.TabRestore {
		b.RestoreState()
	}

	if !b.quietStealthObservers() {
		b.MonitorCrashes(nil)
	}

	return nil
}

// RestartBrowser performs a soft restart: drains in-flight requests, tears
// down Chrome contexts, and re-initializes via EnsureChrome.
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

// Cleanup releases browser resources and removes temporary profile directories.
// Must be called on shutdown to prevent Chrome process and disk leaks.
func (b *Bridge) Cleanup() {
	if b.TabManager != nil && b.tempProfileDir == "" {
		b.SaveState()
	}

	// Mark a clean exit so Chrome doesn't show a crash recovery bar
	if b.Config != nil && b.tempProfileDir == "" {
		MarkCleanExit(b.Config.ProfileDir)
	}

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

	if b.Config != nil && b.TabManager == nil {
		if b.IdMgr == nil {
			b.IdMgr = ids.NewManager()
		}
		if b.LogStore == nil {
			b.LogStore = NewConsoleLogStore(1000)
		}
		b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.LogStore, b.tabSetup)
		b.SetOnAfterClose(func() { go b.SaveState() })
		b.SetDialogManager(b.Dialogs)
		b.SetNetworkMonitor(b.netMonitor)
	}
}

func cryptoRandSeed() int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000000))
	if err != nil {
		return 42
	}
	return n.Int64()
}
