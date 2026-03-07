package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/profiles"
	"github.com/pinchtab/pinchtab/internal/proxy"
	"github.com/pinchtab/pinchtab/internal/scheduler"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/web"

	// Register strategies via init().
	_ "github.com/pinchtab/pinchtab/internal/strategy/autorestart"
	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit"
	_ "github.com/pinchtab/pinchtab/internal/strategy/simple"
)

// runDashboard starts a lightweight dashboard server — no Chrome, no bridge.
// It manages PinchTab instances via the orchestrator and serves the dashboard UI.
func runDashboard(cfg *config.RuntimeConfig) {
	dashPort := cfg.Port
	if dashPort == "" {
		dashPort = "9870"
	}
	startedAt := time.Now()

	slog.Info("🦀 PinchTab", "port", dashPort)

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
	configAPI := dashboard.NewConfigAPI(cfg, orch, profMgr, orch, version, startedAt)

	// Wire up instance events to SSE broadcast
	orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		dash.BroadcastSystemEvent(dashboard.SystemEvent{
			Type:     evt.Type,
			Instance: evt.Instance,
		})
	})

	mux := http.NewServeMux()

	dash.RegisterHandlers(mux)
	configAPI.RegisterHandlers(mux)
	profMgr.RegisterHandlers(mux)

	// Strategy-based routing: if a known strategy is configured, let it handle
	// instance management and shorthand route registration. Otherwise fall back
	// to the default manual route setup for backward compatibility.
	var activeStrategy strategy.Strategy
	if cfg.Strategy != "" {
		strat, err := strategy.New(cfg.Strategy)
		if err != nil {
			slog.Warn("unknown strategy, falling back to default", "strategy", cfg.Strategy, "err", err)
		} else {
			// Inject orchestrator dependency.
			if setter, ok := strat.(strategy.OrchestratorAware); ok {
				setter.SetOrchestrator(orch)
			}
			strat.RegisterRoutes(mux)
			activeStrategy = strat
			slog.Info("strategy activated", "strategy", strat.Name())
		}
	}

	// If no strategy handled route registration, register default routes.
	if activeStrategy == nil {
		orch.RegisterHandlers(mux)
		registerDefaultProxyRoutes(mux, orch)
	}

	// Scheduler: opt-in task queue for multi-agent coordination.
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
		web.JSON(w, 200, map[string]any{"metrics": handlers.SnapshotMetrics()})
	})

	handler := handlers.LoggingMiddleware(handlers.CorsMiddleware(handlers.AuthMiddleware(cfg, mux)))
	if cfg.AllowEvaluate && cfg.Token == "" {
		slog.Warn("evaluate endpoint enabled without API token", "hint", "set PINCHTAB_TOKEN for authenticated access")
	}

	srv := &http.Server{
		Addr:              cfg.Bind + ":" + dashPort,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start strategy lifecycle if active. For strategies like simple-autorestart,
	// Start() launches the initial instance and begins crash monitoring.
	// When a strategy handles auto-launch, skip the manual auto-launch below.
	strategyHandlesLaunch := false
	if activeStrategy != nil {
		if err := activeStrategy.Start(context.Background()); err != nil {
			slog.Error("strategy start failed", "strategy", activeStrategy.Name(), "err", err)
		} else {
			strategyHandlesLaunch = true
		}
	}

	autoLaunch := strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "1") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "true") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "yes")
	if autoLaunch && !strategyHandlesLaunch {
		defaultProfile := os.Getenv("PINCHTAB_DEFAULT_PROFILE")
		defaultProfileExplicit := defaultProfile != ""
		defaultPort := os.Getenv("PINCHTAB_DEFAULT_PORT")

		go func() {
			time.Sleep(500 * time.Millisecond)
			profileToLaunch := defaultProfile
			// If profile is not explicitly configured, prefer an existing profile.
			// Only synthesize "default" when nothing exists yet.
			if !defaultProfileExplicit {
				list, err := profMgr.List()
				if err != nil {
					slog.Warn("auto-launch profile list failed", "err", err)
				}
				if len(list) > 0 {
					profileToLaunch = list[0].Name
				} else {
					profileToLaunch = "default"
					if err := os.MkdirAll(filepath.Join(profilesDir, profileToLaunch, "Default"), 0755); err != nil {
						slog.Warn("failed to create auto-launch profile dir", "profile", profileToLaunch, "err", err)
					}
				}
			}

			headlessDefault := os.Getenv("PINCHTAB_HEADED") == ""
			inst, err := orch.Launch(profileToLaunch, defaultPort, headlessDefault, nil)
			if err != nil {
				slog.Warn("auto-launch failed", "profile", profileToLaunch, "err", err)
				return
			}
			slog.Info("auto-launched instance", "profile", profileToLaunch, "id", inst.ID, "port", inst.Port, "headless", headlessDefault)
		}()
	} else if !strategyHandlesLaunch {
		slog.Info("dashboard auto-launch disabled", "hint", "set PINCHTAB_AUTO_LAUNCH=1 or PINCHTAB_STRATEGY=simple-autorestart to enable")
	}

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down dashboard...")
			if activeStrategy != nil {
				if err := activeStrategy.Stop(); err != nil {
					slog.Warn("strategy stop failed", "err", err)
				}
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
		web.JSON(w, 200, map[string]string{"status": "shutting down"})
		go doShutdown()
	})

	go func() {
		sig := make(chan os.Signal, 2)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		go doShutdown()
		<-sig
		slog.Warn("force shutdown requested")
		orch.ForceShutdown()
		os.Exit(130)
	}()

	// Periodic health check: log tabs and Chrome process info every 30 seconds
	go periodicHealthCheck(orch)

	slog.Info("dashboard ready", "url", fmt.Sprintf("http://localhost:%s", dashPort))

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

// periodicHealthCheck logs instance and Chrome process status every 30 seconds
func periodicHealthCheck(orch *orchestrator.Orchestrator) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Get instance information
		instances := orch.List()
		if len(instances) == 0 {
			continue // No instances running, skip logging
		}

		// Count instances by headedness
		headedCount := 0
		headlessCount := 0

		for _, inst := range instances {
			if inst.Headless {
				headlessCount++
			} else {
				headedCount++
			}
		}

		// Get tabs across all instances
		allTabs := orch.AllTabs()

		slog.Info("health check",
			"instances", len(instances),
			"headed", headedCount,
			"headless", headlessCount,
			"total_tabs", len(allTabs),
		)
	}
}

// registerDefaultProxyRoutes adds the default shorthand proxy routes
// when no strategy is handling route registration.
func registerDefaultProxyRoutes(mux *http.ServeMux, orch *orchestrator.Orchestrator) {
	// Special handler for /tabs — return empty list if no instances.
	mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
		target := orch.FirstRunningURL()
		if target == "" {
			web.JSON(w, 200, map[string]any{"tabs": []any{}})
			return
		}
		proxy.HTTP(w, r, target+"/tabs")
	})

	proxyEndpoints := []string{
		"GET /snapshot", "GET /screenshot", "GET /text",
		"POST /navigate", "POST /action", "POST /actions", "POST /evaluate",
		"POST /tab", "POST /tab/lock", "POST /tab/unlock",
		"GET /cookies", "POST /cookies",
		"GET /download", "POST /upload",
		"GET /stealth/status", "POST /fingerprint/rotate",
		"GET /screencast", "GET /screencast/tabs",
		"POST /find", "POST /macro",
	}
	for _, ep := range proxyEndpoints {
		endpoint := ep
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			target := orch.FirstRunningURL()
			if target == "" {
				web.Error(w, 503, fmt.Errorf("no running instances — launch one from the Profiles tab"))
				return
			}
			path := r.URL.Path
			proxy.HTTP(w, r, target+path)
		})
	}
}
