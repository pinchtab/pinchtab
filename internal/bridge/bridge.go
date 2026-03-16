package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/idutil"
	"github.com/pinchtab/pinchtab/internal/uameta"
)

type TabEntry struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	Accessed  bool
	CDPID     string    // raw CDP target ID
	CreatedAt time.Time // when the tab was first created/registered
	LastUsed  time.Time // last time the tab was accessed via TabContext
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
	IdMgr         *idutil.Manager
	*TabManager
	StealthScript string
	Actions       map[string]ActionFunc
	Locks         *LockManager

	// Lazy initialization
	initMu      sync.Mutex
	initialized bool

	// Temp profile cleanup: directories created as fallback when profile lock fails.
	// These are removed on Cleanup() to prevent Chrome process/disk leaks.
	tempProfileDir string
}

func New(allocCtx, browserCtx context.Context, cfg *config.RuntimeConfig) *Bridge {
	idMgr := idutil.NewManager()
	b := &Bridge{
		AllocCtx:   allocCtx,
		BrowserCtx: browserCtx,
		Config:     cfg,
		IdMgr:      idMgr,
	}
	// Only initialize TabManager if browserCtx is provided (not lazy-init case)
	if cfg != nil && browserCtx != nil {
		b.TabManager = NewTabManager(browserCtx, cfg, idMgr, b.tabSetup)
	}
	b.Locks = NewLockManager()
	b.InitActionRegistry()
	return b
}

func (b *Bridge) injectStealth(ctx context.Context) {
	if b.StealthScript == "" {
		return
	}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(b.StealthScript).Do(ctx)
			return err
		}),
	); err != nil {
		slog.Warn("stealth injection failed", "err", err)
	}
}

func (b *Bridge) tabSetup(ctx context.Context) {
	if override := uameta.Build(b.Config.UserAgent, b.Config.ChromeVersion); override != nil {
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return override.Do(c)
		})); err != nil {
			slog.Warn("ua override failed on tab setup", "err", err)
		}
	}
	b.injectStealth(ctx)
	if b.Config.NoAnimations {
		if err := b.InjectNoAnimations(ctx); err != nil {
			slog.Warn("no-animations injection failed", "err", err)
		}
	}
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

func (b *Bridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	b.initMu.Lock()
	defer b.initMu.Unlock()

	if b.initialized && b.BrowserCtx != nil {
		return nil // Already initialized
	}

	if b.BrowserCtx != nil {
		return nil // Already has browser context
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
	allocCtx, allocCancel, browserCtx, browserCancel, err := InitChrome(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize chrome: %w", err)
	}

	b.AllocCtx = allocCtx
	b.AllocCancel = allocCancel
	b.BrowserCtx = browserCtx
	b.BrowserCancel = browserCancel
	b.initialized = true

	// Initialize TabManager now that browser is ready
	if b.Config != nil && b.TabManager == nil {
		if b.IdMgr == nil {
			b.IdMgr = idutil.NewManager()
		}
		b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.tabSetup)
	}

	// Ensure action registry is populated (idempotent)
	if b.Actions == nil {
		b.InitActionRegistry()
	}

	// Start crash monitoring
	b.MonitorCrashes(nil)

	return nil
}

// Cleanup releases browser resources and removes temporary profile directories.
// Must be called on shutdown to prevent Chrome process and disk leaks.
func (b *Bridge) Cleanup() {
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

	// Now initialize TabManager with the browser context
	if b.Config != nil && b.TabManager == nil {
		if b.IdMgr == nil {
			b.IdMgr = idutil.NewManager()
		}
		b.TabManager = NewTabManager(browserCtx, b.Config, b.IdMgr, b.tabSetup)
	}
}

func (b *Bridge) BrowserContext() context.Context {
	return b.BrowserCtx
}

func (b *Bridge) ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error) {
	fn, ok := b.Actions[kind]
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", kind)
	}
	return fn(ctx, req)
}

// Execute delegates to TabManager.Execute for safe parallel tab execution.
// If TabManager is not initialized, the task runs directly.
func (b *Bridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	if b.TabManager != nil {
		return b.TabManager.Execute(ctx, tabID, task)
	}
	return task(ctx)
}

func (b *Bridge) AvailableActions() []string {
	keys := make([]string, 0, len(b.Actions))
	for k := range b.Actions {
		keys = append(keys, k)
	}
	return keys
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
