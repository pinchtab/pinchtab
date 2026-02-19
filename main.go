package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// version is set by goreleaser via ldflags
var version = "dev"

// chromeStartTimeout is the maximum time to wait for Chrome to connect via CDP.
// If exceeded, Pinchtab clears session data and retries once.
const chromeStartTimeout = 15 * time.Second

var bridge Bridge

func main() {

	loadConfig()

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("pinchtab %s\n", version)
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "config" {
		handleConfigCommand()
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "connect" {
		handleConnectCommand()
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "dashboard" {
		runDashboard()
		return
	}

	if err := os.MkdirAll(cfg.StateDir, 0755); err != nil {
		slog.Error("cannot create state dir", "err", err)
		os.Exit(1)
	}

	allocCtx, allocCancel, chromeOpts := setupAllocator()
	defer allocCancel()

	stealthSeed := rand.Intn(1000000000)
	SetHumanRandSeed(int64(stealthSeed))
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", stealthSeed, cfg.StealthLevel) + stealthScript
	bridge.stealthScript = seededScript
	bridge.allocCtx = allocCtx

	browserCtx, browserCancel, err := startChrome(seededScript)
	if err != nil {
		slog.Warn("Chrome startup failed, clearing sessions and retrying once", "err", err)

		allocCancel()
		clearChromeSessions()
		markCleanExit()

		allocCtx, allocCancel, _ = setupAllocator()
		bridge.allocCtx = allocCtx
		_ = chromeOpts // used implicitly via setupAllocator

		browserCtx, browserCancel, err = startChrome(seededScript)
		if err != nil {
			slog.Error("Chrome failed to start after retry",
				"err", err,
				"hint", "try BRIDGE_NO_RESTORE=true or delete your profile directory",
				"profile", cfg.ProfileDir,
			)
			allocCancel()
			os.Exit(1)
		}
		slog.Info("Chrome started on retry")
	}
	defer browserCancel()

	applyTimezone(browserCtx)

	bridge.browserCtx = browserCtx
	bridge.InitTabManager()
	bridge.initActionRegistry()
	bridge.locks = newLockManager()

	profilesDir := filepath.Join(filepath.Dir(cfg.ProfileDir), "profiles")
	profMgr := NewProfileManager(profilesDir)
	dashboard := NewDashboard(nil)
	orchestrator := NewOrchestrator(profilesDir)

	initTargetID := chromedp.FromContext(browserCtx).Target.TargetID
	bridge.RegisterTab(string(initTargetID), browserCtx)
	slog.Info("initial tab", "id", string(initTargetID))

	if !cfg.Headless {
		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = chromedp.Run(browserCtx, chromedp.Navigate("http://localhost:"+cfg.Port+"/welcome"))
		}()
	}

	if !cfg.NoRestore {
		go bridge.RestoreState()
	}

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go bridge.CleanStaleTabs(cleanupCtx, 30*cfg.ActionTimeout)

	mux := http.NewServeMux()
	registerRoutes(mux, &bridge, profMgr, dashboard, orchestrator)

	profileObserver := func(evt AgentEvent) {
		if evt.Profile != "" {
			profMgr.RecordAction(evt.Profile, ActionRecord{
				Timestamp:  evt.Timestamp,
				Method:     strings.SplitN(evt.Action, " ", 2)[0],
				Endpoint:   strings.SplitN(evt.Action, " ", 2)[1],
				URL:        evt.URL,
				TabID:      evt.TabID,
				DurationMs: evt.DurationMs,
				Status:     evt.Status,
			})
		}
	}

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: dashboard.TrackingMiddleware(
		[]EventObserver{profileObserver},
		loggingMiddleware(corsMiddleware(authMiddleware(mux))),
	)}

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down, saving state...")
			orchestrator.Shutdown()
			cleanupCancel()
			bridge.SaveState()
			markCleanExit()

			shutdownCtx, shutdownDone := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer shutdownDone()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Error("shutdown http", "err", err)
			}

			browserCancel()
			allocCancel()
			slog.Info("chrome closed")
		})
	}

	mux.HandleFunc("POST /shutdown", bridge.handleShutdown(doShutdown))

	setupSignalHandler(doShutdown, func() {
		orchestrator.ForceShutdown()
		cleanupCancel()
		browserCancel()
		allocCancel()
	})

	slog.Info("🦀 PINCH! PINCH!", "port", cfg.Port, "cdp", cfg.CdpURL, "stealth", cfg.StealthLevel)
	if cfg.Token != "" {
		slog.Info("auth enabled")
	} else {
		slog.Info("auth disabled (set BRIDGE_TOKEN to enable)")
	}

	go runStartupHealthCheck()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

// setupAllocator creates the Chrome allocator context (remote or local).
func setupAllocator() (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption) {
	if cfg.CdpURL != "" {
		slog.Info("connecting to Chrome", "url", cfg.CdpURL)
		ctx, cancel := chromedp.NewRemoteAllocator(context.Background(), cfg.CdpURL)
		return ctx, cancel, nil
	}

	if err := os.MkdirAll(cfg.ProfileDir, 0755); err != nil {
		slog.Error("cannot create profile dir", "err", err)
		os.Exit(1)
	}

	for _, lockName := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		lockPath := fmt.Sprintf("%s/%s", cfg.ProfileDir, lockName)
		if err := os.Remove(lockPath); err == nil {
			slog.Warn("removed stale lock", "file", lockName)
		}
	}

	if wasUncleanExit() {
		slog.Warn("previous session exited uncleanly, clearing Chrome session restore data")
		clearChromeSessions()
	}

	slog.Info("launching Chrome", "profile", cfg.ProfileDir, "headless", cfg.Headless)

	opts := buildChromeOpts()

	markCleanExit()
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return ctx, cancel, opts
}

// buildChromeOpts assembles the Chrome flags for local execution.
func buildChromeOpts() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.UserDataDir(cfg.ProfileDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,

		chromedp.Flag("exclude-switches", "enable-automation"),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-web-security", false),

		chromedp.Flag("disable-background-networking", false),
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("no-first-run", true),

		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.Flag("hide-crash-restore-bubble", true),
		chromedp.Flag("disable-device-discovery-notifications", true),

		chromedp.Flag("js-flags", "--random-seed=1157259157"),

		chromedp.WindowSize(1366, 768),
	}

	if cfg.ChromeBinary != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ChromeBinary))
	}
	if cfg.ChromeExtraFlags != "" {
		for _, f := range strings.Fields(cfg.ChromeExtraFlags) {
			if k, v, ok := strings.Cut(f, "="); ok {
				opts = append(opts, chromedp.Flag(strings.TrimLeft(k, "-"), v))
			} else {
				opts = append(opts, chromedp.Flag(strings.TrimLeft(f, "-"), true))
			}
		}
	}

	if cfg.Headless {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	return opts
}

// startChrome creates a browser context and injects the stealth script.
func startChrome(seededScript string) (context.Context, context.CancelFunc, error) {
	bCtx, bCancel := chromedp.NewContext(bridge.allocCtx)

	startCtx, startDone := context.WithTimeout(context.Background(), chromeStartTimeout)
	defer startDone()

	errCh := make(chan error, 1)
	go func() {
		errCh <- chromedp.Run(bCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := page.AddScriptToEvaluateOnNewDocument(seededScript).Do(ctx)
				return err
			}),
		)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			bCancel()
			return nil, nil, err
		}
		return bCtx, bCancel, nil
	case <-startCtx.Done():
		bCancel()
		return nil, nil, fmt.Errorf("timed out after %s", chromeStartTimeout)
	}
}

// applyTimezone sets the browser timezone override if configured.
func applyTimezone(browserCtx context.Context) {
	if cfg.Timezone == "" {
		return
	}
	if err := chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetTimezoneOverride(cfg.Timezone).Do(ctx)
		}),
	); err != nil {
		slog.Warn("timezone override failed", "tz", cfg.Timezone, "err", err)
	} else {
		slog.Info("timezone override", "tz", cfg.Timezone)
	}
}

// registerRoutes wires all HTTP handlers to the mux.
func registerRoutes(mux *http.ServeMux, b *Bridge, profMgr ProfileService, dashboard *Dashboard, orchestrator OrchestratorService) {
	mux.HandleFunc("GET /health", b.handleHealth)
	mux.HandleFunc("GET /tabs", b.handleTabs)
	mux.HandleFunc("GET /snapshot", b.handleSnapshot)
	mux.HandleFunc("GET /screenshot", b.handleScreenshot)
	mux.HandleFunc("GET /text", b.handleText)
	mux.HandleFunc("POST /navigate", b.handleNavigate)
	mux.HandleFunc("POST /action", b.handleAction)
	mux.HandleFunc("POST /actions", b.handleActions)
	mux.HandleFunc("POST /evaluate", b.handleEvaluate)
	mux.HandleFunc("POST /tab", b.handleTab)
	mux.HandleFunc("POST /tab/lock", b.handleTabLock)
	mux.HandleFunc("POST /tab/unlock", b.handleTabUnlock)
	mux.HandleFunc("GET /cookies", b.handleGetCookies)
	mux.HandleFunc("POST /cookies", b.handleSetCookies)
	mux.HandleFunc("GET /stealth/status", b.handleStealthStatus)
	mux.HandleFunc("POST /fingerprint/rotate", b.handleFingerprintRotate)
	mux.HandleFunc("GET /screencast", b.handleScreencast)
	mux.HandleFunc("GET /screencast/tabs", b.handleScreencastAll)
	mux.HandleFunc("GET /welcome", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(welcomeHTML))
	})

	profMgr.RegisterHandlers(mux)
	dashboard.RegisterHandlers(mux)
	if os.Getenv("BRIDGE_NO_DASHBOARD") == "" {
		orchestrator.RegisterHandlers(mux)
	}
}

// setupSignalHandler listens for SIGINT/SIGTERM for graceful and force shutdown.
func setupSignalHandler(shutdownFn func(), forceFn func()) {
	go func() {
		sig := make(chan os.Signal, 2)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		go shutdownFn()
		<-sig
		slog.Warn("force shutdown requested")
		forceFn()
		os.Exit(130)
	}()
}

// runStartupHealthCheck verifies the server is responding after launch.
func runStartupHealthCheck() {
	time.Sleep(500 * time.Millisecond)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%s/health", cfg.Port))
	if err != nil {
		slog.Error("startup health check failed",
			"err", err,
			"hint", "try BRIDGE_NO_RESTORE=true or delete your profile directory",
		)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		slog.Info("startup health check passed")
	} else {
		slog.Warn("startup health check unexpected status", "status", resp.StatusCode)
	}
}
