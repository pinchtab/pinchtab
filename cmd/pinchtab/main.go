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

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/human"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

var commonWindowSizes = [][2]int{
	{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
	{1280, 720}, {1600, 900}, {2560, 1440}, {1280, 800},
}

func randomWindowSize() (int, int) {
	s := commonWindowSizes[rand.Intn(len(commonWindowSizes))]
	return s[0], s[1]
}

var version = "dev"

const chromeStartTimeout = 15 * time.Second

func main() {
	cfg := config.Load()

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("pinchtab %s\n", version)
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "config" {
		config.HandleConfigCommand(cfg)
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "connect" {
		handleConnectCommand(cfg)
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "dashboard" {
		runDashboard(cfg)
		return
	}

	if err := os.MkdirAll(cfg.StateDir, 0755); err != nil {
		slog.Error("cannot create state dir", "err", err)
		os.Exit(1)
	}

	allocCtx, allocCancel, chromeOpts := setupAllocator(cfg)
	defer allocCancel()

	stealthSeed := rand.Intn(1000000000)
	human.SetHumanRandSeed(int64(stealthSeed))
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", stealthSeed, cfg.StealthLevel) + assets.StealthScript

	browserCtx, browserCancel, err := startChrome(allocCtx, cfg, seededScript)
	if err != nil {
		slog.Warn("Chrome startup failed, clearing sessions and retrying once", "err", err)

		allocCancel()
		bridge.ClearChromeSessions(cfg.ProfileDir)
		bridge.MarkCleanExit(cfg.ProfileDir)

		allocCtx, allocCancel, _ = setupAllocator(cfg)
		_ = chromeOpts // used implicitly via setupAllocator

		browserCtx, browserCancel, err = startChrome(allocCtx, cfg, seededScript)
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

	applyTimezone(browserCtx, cfg)
	applyUserAgentOverride(browserCtx, cfg)

	b := bridge.New(allocCtx, browserCtx, cfg)
	b.StealthScript = seededScript
	b.InitActionRegistry()

	profilesDir := filepath.Join(filepath.Dir(cfg.ProfileDir), "profiles")
	profMgr := profiles.NewProfileManager(profilesDir)
	dash := dashboard.NewDashboard(nil)
	orch := orchestrator.NewOrchestrator(profilesDir)
	orch.SetProfileManager(profMgr)
	dash.SetInstanceLister(orch)

	// For CDP_URL mode, the initial target might not exist yet.
	// Tabs will be registered when they're created or discovered.
	if cfg.CdpURL == "" {
		initTargetID := chromedp.FromContext(browserCtx).Target.TargetID
		b.RegisterTab(string(initTargetID), browserCtx)
		slog.Info("initial tab", "id", string(initTargetID))
	} else {
		slog.Info("CDP_URL mode: skipping initial tab registration")
	}

	if !cfg.Headless {
		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = chromedp.Run(browserCtx, chromedp.Navigate("http://localhost:"+cfg.Port+"/welcome"))
		}()
	}

	if !cfg.NoRestore {
		go b.RestoreState()
	}

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go b.CleanStaleTabs(cleanupCtx, 30*cfg.ActionTimeout)

	mux := http.NewServeMux()
	h := handlers.New(b, cfg, profMgr, dash, orch)

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down, saving state...")
			orch.Shutdown()
			cleanupCancel()
			b.SaveState()
			bridge.MarkCleanExit(cfg.ProfileDir)

			browserCancel()
			allocCancel()
			slog.Info("chrome closed")
		})
	}

	h.RegisterRoutes(mux, doShutdown)

	profileObserver := func(evt dashboard.AgentEvent) {
		if evt.Profile != "" {
			profMgr.RecordAction(evt.Profile, bridge.ActionRecord{
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

	handler := dash.TrackingMiddleware(
		[]dashboard.EventObserver{profileObserver},
		handlers.LoggingMiddleware(handlers.CorsMiddleware(handlers.AuthMiddleware(cfg, mux))),
	)

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	setupSignalHandler(doShutdown, func() {
		orch.ForceShutdown()
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

	go runStartupHealthCheck(cfg)

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

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

func runStartupHealthCheck(cfg *config.RuntimeConfig) {
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
