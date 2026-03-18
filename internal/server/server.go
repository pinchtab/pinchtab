package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/profiles"
	"github.com/pinchtab/pinchtab/internal/scheduler"
	"github.com/pinchtab/pinchtab/internal/strategy"

	// Register strategies
	_ "github.com/pinchtab/pinchtab/internal/strategy/alwayson"
	_ "github.com/pinchtab/pinchtab/internal/strategy/autorestart"
	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit"
	_ "github.com/pinchtab/pinchtab/internal/strategy/simple"
)

func RunDashboard(cfg *config.RuntimeConfig, version string) {
	// Clean up orphaned Chrome processes from previous crashed runs
	bridge.CleanupOrphanedChromeProcesses(cfg.ProfileDir)

	dashPort := cfg.Port
	if dashPort == "" {
		dashPort = "9870"
	}
	startedAt := time.Now()

	profilesDir := cfg.ProfilesBaseDir
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		slog.Error("cannot create profiles dir", "err", err)
		os.Exit(1)
	}

	profMgr := profiles.NewProfileManager(profilesDir)
	dash := dashboard.NewDashboard(nil)
	orch := orchestrator.NewOrchestrator(profilesDir)
	orch.ApplyRuntimeConfig(cfg)
	orch.SetProfileManager(profMgr)
	dash.SetInstanceLister(orch)
	dash.SetMonitoringSource(orch)
	dash.SetServerMetricsProvider(func() dashboard.MonitoringServerMetrics {
		snapshot := handlers.SnapshotMetrics()
		return dashboard.MonitoringServerMetrics{
			GoHeapAllocMB:   MetricFloat(snapshot["goHeapAllocMB"]),
			GoNumGoroutine:  MetricInt(snapshot["goNumGoroutine"]),
			RateBucketHosts: MetricInt(snapshot["rateBucketHosts"]),
		}
	})
	configAPI := dashboard.NewConfigAPI(cfg, orch, profMgr, orch, version, startedAt)
	sessions := authn.NewSessionManager(authn.SessionConfig{
		IdleTimeout: authn.DefaultSessionIdleTimeout,
		MaxLifetime: authn.DefaultSessionMaxLifetime,
	})
	authAPI := dashboard.NewAuthAPI(cfg, sessions)

	// Wire up instance events to SSE broadcast
	orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		dash.BroadcastSystemEvent(dashboard.SystemEvent{
			Type:     evt.Type,
			Instance: evt.Instance,
		})
	})
	actStore, err := activity.NewRecorder(activity.Config{
		Enabled:       cfg.Observability.Activity.Enabled,
		SessionIdle:   time.Duration(cfg.Observability.Activity.SessionIdleSec) * time.Second,
		RetentionDays: cfg.Observability.Activity.RetentionDays,
	}, cfg.StateDir)
	if err != nil {
		slog.Error("activity store", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	dash.RegisterHandlers(mux)
	configAPI.RegisterHandlers(mux)
	authAPI.RegisterHandlers(mux)
	profMgr.RegisterHandlers(mux)
	activity.RegisterHandlers(mux, actStore)

	strategyName := cfg.Strategy
	if strategyName == "" {
		strategyName = "always-on"
	}
	activeStrategy, err := strategy.New(strategyName)
	if err != nil {
		slog.Warn("unknown strategy, falling back to always-on", "strategy", strategyName, "err", err)
		activeStrategy, err = strategy.New("always-on")
		if err != nil {
			slog.Error("failed to initialize fallback strategy", "strategy", "always-on", "err", err)
			os.Exit(1)
		}
	}
	if runtimeAware, ok := activeStrategy.(strategy.RuntimeConfigAware); ok {
		runtimeAware.SetRuntimeConfig(cfg)
	}
	if setter, ok := activeStrategy.(strategy.OrchestratorAware); ok {
		setter.SetOrchestrator(orch)
	}
	activeStrategy.RegisterRoutes(mux)
	stratName := activeStrategy.Name()

	allocPolicy := cfg.AllocationPolicy
	if allocPolicy == "" {
		allocPolicy = "none"
	}

	listenStatus := "starting"
	if cli.IsDaemonRunning() && CheckPinchTabRunning(dashPort, cfg.Token) {
		listenStatus = "running"
	}

	cli.PrintStartupBanner(cfg, cli.StartupBannerOptions{
		Mode:         "server",
		ListenAddr:   cfg.Bind + ":" + dashPort,
		ListenStatus: listenStatus,
		PublicURL:    fmt.Sprintf("http://localhost:%s", dashPort),
		Strategy:     stratName,
		Allocation:   allocPolicy,
	})

	if listenStatus == "running" {
		fmt.Println(cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("  pinchtab already running as a daemon on port %s", dashPort)))
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "  Stop the daemon first with `pinchtab daemon stop` to run in the foreground."))
		fmt.Println()
		os.Exit(0)
	}

	slog.Info("orchestration", "strategy", stratName, "allocation", allocPolicy)

	var sched *scheduler.Scheduler
	if cfg.Scheduler.Enabled {
		schedCfg := scheduler.DefaultConfig()
		schedCfg.Enabled = true
		if cfg.Scheduler.Strategy != "" {
			schedCfg.Strategy = cfg.Scheduler.Strategy
		}
		if cfg.Scheduler.MaxQueueSize > 0 {
			schedCfg.MaxQueueSize = cfg.Scheduler.MaxQueueSize
		}
		if cfg.Scheduler.MaxPerAgent > 0 {
			schedCfg.MaxPerAgent = cfg.Scheduler.MaxPerAgent
		}
		if cfg.Scheduler.MaxInflight > 0 {
			schedCfg.MaxInflight = cfg.Scheduler.MaxInflight
		}
		if cfg.Scheduler.MaxPerAgentFlight > 0 {
			schedCfg.MaxPerAgentFlight = cfg.Scheduler.MaxPerAgentFlight
		}
		if cfg.Scheduler.ResultTTLSec > 0 {
			schedCfg.ResultTTL = time.Duration(cfg.Scheduler.ResultTTLSec) * time.Second
		}
		if cfg.Scheduler.WorkerCount > 0 {
			schedCfg.WorkerCount = cfg.Scheduler.WorkerCount
		}

		resolver := &scheduler.ManagerResolver{Mgr: orch.InstanceManager()}
		sched = scheduler.New(schedCfg, resolver)
		sched.RegisterHandlers(mux)
		sched.Start()
		slog.Info("scheduler enabled", "strategy", schedCfg.Strategy, "workers", schedCfg.WorkerCount)
	}

	mux.HandleFunc("GET /health", configAPI.HandleHealth)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"metrics": handlers.SnapshotMetrics()})
	})

	handler := handlers.RequestIDMiddleware(
		activity.Middleware(
			actStore,
			"server",
			handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.CorsMiddleware(cfg, handlers.AuthMiddlewareWithSessions(cfg, sessions, mux)))),
		),
	)
	cli.LogSecurityWarnings(cfg)

	srv := &http.Server{
		Addr:              cfg.Bind + ":" + dashPort,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := activeStrategy.Start(context.Background()); err != nil {
		slog.Error("strategy start failed", "strategy", activeStrategy.Name(), "err", err)
	}

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down dashboard...")
			// Kill all Chrome processes under our profiles dir immediately.
			// This runs before strategy.Stop() to ensure cleanup happens
			// even if launchd SIGKILL arrives during graceful shutdown.
			bridge.KillAllPinchtabChrome()
			if err := activeStrategy.Stop(); err != nil {
				slog.Warn("strategy stop failed", "err", err)
			}
			if sched != nil {
				sched.Stop()
			}
			dash.Shutdown()
			orch.Shutdown()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				slog.Error("shutdown http", "err", err)
			}
		})
	}

	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		authn.AuditLog(r, "system.shutdown_requested")
		httpx.JSON(w, 200, map[string]string{"status": "shutting down"})
		go doShutdown()
	})

	go func() {
		sig := make(chan os.Signal, 2)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		// Kill Chrome immediately on signal — synchronous, before anything else.
		// launchd may SIGKILL us shortly after SIGTERM, so this must happen first.
		bridge.KillAllPinchtabChrome()
		go doShutdown()
		<-sig
		slog.Warn("force shutdown requested")
		orch.ForceShutdown()
		os.Exit(130)
	}()

	slog.Info("dashboard started", "port", dashPort)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
