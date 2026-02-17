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
	// Load configuration from file or environment
	loadConfig()

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("pinchtab %s\n", version)
		os.Exit(0)
	}

	// Handle config generation
	if len(os.Args) > 1 && os.Args[1] == "config" {
		handleConfigCommand()
		os.Exit(0)
	}

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		slog.Error("cannot create state dir", "err", err)
		os.Exit(1)
	}

	var allocCancel context.CancelFunc
	var chromeOpts []chromedp.ExecAllocatorOption // stored for retry

	if cdpURL != "" {
		slog.Info("connecting to Chrome", "url", cdpURL)
		bridge.allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), cdpURL)
	} else {
		if err := os.MkdirAll(profileDir, 0755); err != nil {
			slog.Error("cannot create profile dir", "err", err)
			os.Exit(1)
		}

		// Remove stale Chrome lock files from unclean shutdowns.
		for _, lockName := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
			lockPath := fmt.Sprintf("%s/%s", profileDir, lockName)
			if err := os.Remove(lockPath); err == nil {
				slog.Warn("removed stale lock", "file", lockName)
			}
		}

		// Detect unclean previous exit and clear Chrome's session restore data
		// to prevent hanging on restored tabs that fail to load.
		if wasUncleanExit() {
			slog.Warn("previous session exited uncleanly, clearing Chrome session restore data")
			clearChromeSessions()
		}

		slog.Info("launching Chrome", "profile", profileDir, "headless", headless)

		chromeOpts = []chromedp.ExecAllocatorOption{
			// Profile & basics
			chromedp.UserDataDir(profileDir),
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,

			// Advanced stealth: hide automation indicators
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

			// Performance & networking
			chromedp.Flag("disable-background-networking", false),
			chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
			chromedp.Flag("disable-popup-blocking", true),
			chromedp.Flag("no-first-run", true),

			// UI: suppress crash bar and notifications
			chromedp.Flag("disable-session-crashed-bubble", true),
			chromedp.Flag("hide-crash-restore-bubble", true),
			chromedp.Flag("disable-device-discovery-notifications", true),

			// Random seed for consistent behavior across runs
			chromedp.Flag("js-flags", "--random-seed=1157259157"),

			// Window size (UA left as Chrome default to avoid detection mismatches)
			chromedp.WindowSize(1366, 768),
		}

		if chromeBinary != "" {
			chromeOpts = append(chromeOpts, chromedp.ExecPath(chromeBinary))
		}
		if chromeExtraFlags != "" {
			for _, f := range strings.Fields(chromeExtraFlags) {
				if k, v, ok := strings.Cut(f, "="); ok {
					chromeOpts = append(chromeOpts, chromedp.Flag(strings.TrimLeft(k, "-"), v))
				} else {
					chromeOpts = append(chromeOpts, chromedp.Flag(strings.TrimLeft(f, "-"), true))
				}
			}
		}

		if headless {
			chromeOpts = append(chromeOpts, chromedp.Headless)
		} else {
			chromeOpts = append(chromeOpts, chromedp.Flag("headless", false))
		}

		markCleanExit()
		bridge.allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), chromeOpts...)
	}
	defer allocCancel()

	// Prepare stealth script
	stealthSeed := rand.Intn(1000000000)
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", stealthSeed, stealthLevel) + stealthScript
	bridge.stealthScript = seededScript

	// startChrome connects to Chrome with a timeout. Returns browserCtx and cancel,
	// or an error if Chrome doesn't respond within chromeStartTimeout.
	startChrome := func() (context.Context, context.CancelFunc, error) {
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

	browserCtx, browserCancel, err := startChrome()
	if err != nil {
		slog.Warn("Chrome startup failed, clearing sessions and retrying once", "err", err)

		// Kill the hung Chrome process and clean up.
		allocCancel()
		clearChromeSessions()
		markCleanExit()

		// Re-create allocator for retry.
		if cdpURL != "" {
			bridge.allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), cdpURL)
		} else {
			bridge.allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), chromeOpts...)
		}

		// Retry once.
		browserCtx, browserCancel, err = startChrome()
		if err != nil {
			slog.Error("Chrome failed to start after retry",
				"err", err,
				"hint", "try BRIDGE_NO_RESTORE=true or delete your profile directory",
				"profile", profileDir,
			)
			allocCancel()
			os.Exit(1)
		}
		slog.Info("Chrome started on retry")
	}
	defer browserCancel()

	// CDP-level timezone override (more reliable than JS-only approach)
	if timezone != "" {
		if err := chromedp.Run(browserCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return emulation.SetTimezoneOverride(timezone).Do(ctx)
			}),
		); err != nil {
			slog.Warn("timezone override failed", "tz", timezone, "err", err)
		} else {
			slog.Info("timezone override", "tz", timezone)
		}
	}

	bridge.browserCtx = browserCtx
	bridge.tabs = make(map[string]*TabEntry)
	bridge.snapshots = make(map[string]*refCache)
	bridge.initActionRegistry()
	bridge.locks = newLockManager()

	// Profile manager + dashboard
	profilesDir := filepath.Join(filepath.Dir(profileDir), "profiles")
	profMgr := NewProfileManager(profilesDir)
	dashboard := NewDashboard()

	// Register the initial tab
	initTargetID := chromedp.FromContext(browserCtx).Target.TargetID
	bridge.tabs[string(initTargetID)] = &TabEntry{ctx: browserCtx}
	slog.Info("initial tab", "id", string(initTargetID))

	// Navigate initial tab to welcome page in headed mode (after server starts)
	if !headless {
		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = chromedp.Run(browserCtx, chromedp.Navigate("http://localhost:"+port+"/welcome"))
		}()
	}

	if !noRestore {
		// Restore in background so it doesn't block the HTTP server
		go bridge.RestoreState()
	}

	// Background tab cleanup
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go bridge.CleanStaleTabs(cleanupCtx, 30*actionTimeout)

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", bridge.handleHealth)
	mux.HandleFunc("GET /tabs", bridge.handleTabs)
	mux.HandleFunc("GET /snapshot", bridge.handleSnapshot)
	mux.HandleFunc("GET /screenshot", bridge.handleScreenshot)
	mux.HandleFunc("GET /text", bridge.handleText)
	mux.HandleFunc("POST /navigate", bridge.handleNavigate)
	mux.HandleFunc("POST /action", bridge.handleAction)
	mux.HandleFunc("POST /actions", bridge.handleActions)
	mux.HandleFunc("POST /evaluate", bridge.handleEvaluate)
	mux.HandleFunc("POST /tab", bridge.handleTab)
	mux.HandleFunc("POST /tab/lock", bridge.handleTabLock)
	mux.HandleFunc("POST /tab/unlock", bridge.handleTabUnlock)
	mux.HandleFunc("GET /cookies", bridge.handleGetCookies)
	mux.HandleFunc("POST /cookies", bridge.handleSetCookies)
	mux.HandleFunc("GET /stealth/status", bridge.handleStealthStatus)
	mux.HandleFunc("POST /fingerprint/rotate", bridge.handleFingerprintRotate)
	mux.HandleFunc("GET /welcome", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(welcomeHTML))
	})

	// Profile management + dashboard
	profMgr.RegisterHandlers(mux)
	dashboard.RegisterHandlers(mux)

	srv := &http.Server{Addr: ":" + port, Handler: dashboard.TrackingMiddleware(profMgr, loggingMiddleware(corsMiddleware(authMiddleware(mux))))}

	// Shutdown orchestration — used by both signal handler and /shutdown endpoint.
	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down, saving state...")
			cleanupCancel()
			bridge.SaveState()
			markCleanExit()

			// Shut down HTTP server first so no new requests come in.
			shutdownCtx, shutdownDone := context.WithTimeout(context.Background(), shutdownTimeout)
			defer shutdownDone()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Error("shutdown http", "err", err)
			}

			// Explicitly close Chrome by canceling the browser and allocator contexts.
			browserCancel()
			allocCancel()
			slog.Info("chrome closed")
		})
	}

	// Wire up /shutdown endpoint (requires auth like all other endpoints).
	mux.HandleFunc("POST /shutdown", bridge.handleShutdown(doShutdown))

	// Signal handler.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		doShutdown()
	}()

	slog.Info("🦀 PINCH! PINCH!", "port", port, "cdp", cdpURL, "stealth", stealthLevel)
	if token != "" {
		slog.Info("auth enabled")
	} else {
		slog.Info("auth disabled (set BRIDGE_TOKEN to enable)")
	}

	// Startup self-check: verify the server can respond within 5s.
	go func() {
		time.Sleep(500 * time.Millisecond) // let ListenAndServe bind
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://localhost:%s/health", port))
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
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
