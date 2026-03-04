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
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/strategy/session"
	"github.com/pinchtab/pinchtab/internal/strategy/simple"
	"github.com/pinchtab/pinchtab/internal/web"

	_ "github.com/pinchtab/pinchtab/internal/strategy/explicit" // register explicit strategy
)

// runDashboard starts a lightweight dashboard server — no Chrome, no bridge.
// It manages PinchTab instances via the orchestrator and serves the dashboard UI.
func runDashboard(cfg *config.RuntimeConfig) {
	dashPort := cfg.Port
	if dashPort == "" {
		dashPort = "9870"
	}

	slog.Info("🦀 PinchTab", "port", dashPort)

	profilesDir := filepath.Join(cfg.StateDir, "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		slog.Error("cannot create profiles dir", "err", err)
		os.Exit(1)
	}

	profMgr := profiles.NewProfileManager(profilesDir)
	dash := dashboard.NewDashboard(nil)
	orch := orchestrator.NewOrchestrator(profilesDir)
	orch.SetProfileManager(profMgr)
	orch.SetPortRange(cfg.InstancePortStart, cfg.InstancePortEnd)
	dash.SetInstanceLister(orch)

	// Wire up instance events to SSE broadcast
	orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		dash.BroadcastSystemEvent(dashboard.SystemEvent{
			Type:     evt.Type,
			Instance: evt.Instance,
		})
	})

	// Set allocation policy if configured.
	if cfg.AllocationPolicy != "" && cfg.AllocationPolicy != "fcfs" {
		if err := orch.SetAllocationPolicy(cfg.AllocationPolicy); err != nil {
			slog.Warn("invalid allocation policy, using fcfs", "policy", cfg.AllocationPolicy, "err", err)
		}
	}

	mux := http.NewServeMux()

	dash.RegisterHandlers(mux)
	// When a strategy is configured, skip orchestrator's tab proxy routes
	// to avoid conflicts — the strategy registers those itself.
	if cfg.Strategy != "" {
		orch.RegisterHandlersWithStrategy(mux)
	} else {
		orch.RegisterHandlers(mux)
	}
	profMgr.RegisterHandlers(mux)

	// Root returns health check (API-first design)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"status":    "ok",
			"dashboard": "/dashboard",
			"docs":      "/api/docs",
		}
		if cfg.Strategy != "" {
			resp["strategy"] = cfg.Strategy
		}
		web.JSON(w, 200, resp)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]string{"status": "ok"}
		if cfg.Strategy != "" {
			resp["strategy"] = cfg.Strategy
		}
		web.JSON(w, 200, resp)
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		web.JSON(w, 200, map[string]any{"metrics": handlers.SnapshotMetrics()})
	})

	// Initialize allocation strategy if configured.
	// When a strategy is active, it registers shorthand endpoints (/navigate, /snapshot, etc.)
	// instead of the legacy proxy-to-first-instance behavior.
	strategyActive := false
	if cfg.Strategy != "" {
		var activeStrategy strategy.Strategy
		mgr := orch.InstanceManager()

		switch cfg.Strategy {
		case "simple":
			activeStrategy = simple.New(mgr)
		case "session":
			activeStrategy = session.New(mgr)
		case "explicit":
			s, err := strategy.New("explicit")
			if err != nil {
				slog.Error("failed to create explicit strategy", "err", err)
				os.Exit(1)
			}
			activeStrategy = s
		default:
			slog.Error("unknown strategy", "strategy", cfg.Strategy,
				"available", "simple, session, explicit")
			os.Exit(1)
		}

		if err := activeStrategy.Init(nil); err != nil {
			slog.Error("strategy init failed", "err", err)
			os.Exit(1)
		}

		activeStrategy.RegisterRoutes(mux)
		strategyActive = true

		// Simple strategy: auto-launch one instance so it's ready immediately.
		if cfg.Strategy == "simple" {
			go func() {
				inst, err := orch.Launch(fmt.Sprintf("instance-%d", time.Now().UnixNano()), "", true)
				if err != nil {
					slog.Error("simple strategy: failed to auto-launch instance", "err", err)
					return
				}
				slog.Info("simple strategy: instance ready", "id", inst.ID, "port", inst.Port)
			}()
		}

		slog.Info("strategy active",
			"strategy", activeStrategy.Name(),
			"policy", cfg.AllocationPolicy,
		)
	}

	// Legacy shorthand proxy endpoints — only registered when no strategy is active.
	// These proxy to the first running instance (simple forwarding, no allocation policy).
	if !strategyActive {
		// Special handler for /tabs - return empty list if no instances
		mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
			target := orch.FirstRunningURL()
			if target == "" {
				web.JSON(w, 200, map[string]interface{}{"tabs": []interface{}{}})
				return
			}
			proxyRequest(w, r, target+"/tabs")
		})

		proxyEndpoints := []string{
			"GET /snapshot", "GET /screenshot", "GET /text",
			"POST /navigate", "POST /action", "POST /actions", "POST /evaluate",
			"POST /find",
			"POST /tab", "POST /tab/lock", "POST /tab/unlock",
			"GET /cookies", "POST /cookies",
			"GET /download", "POST /upload",
			"GET /stealth/status", "POST /fingerprint/rotate",
			"GET /screencast", "GET /screencast/tabs",
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
				proxyRequest(w, r, target+path)
			})
		}
	}

	handler := handlers.LoggingMiddleware(handlers.CorsMiddleware(handlers.AuthMiddleware(cfg, mux)))

	srv := &http.Server{
		Addr:              cfg.Bind + ":" + dashPort,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	autoLaunch := strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "1") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "true") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "yes")
	if autoLaunch {
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
			inst, err := orch.Launch(profileToLaunch, defaultPort, headlessDefault)
			if err != nil {
				slog.Warn("auto-launch failed", "profile", profileToLaunch, "err", err)
				return
			}
			slog.Info("auto-launched instance", "profile", profileToLaunch, "id", inst.ID, "port", inst.Port, "headless", headlessDefault)
		}()
	} else {
		slog.Info("dashboard auto-launch disabled", "hint", "set PINCHTAB_AUTO_LAUNCH=1 to enable")
	}

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down dashboard...")
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

// proxyRequest forwards an HTTP request to a target URL.
// For WebSocket upgrades (screencast), it does a WebSocket proxy.
func proxyRequest(w http.ResponseWriter, r *http.Request, targetURL string) {
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	if isWebSocketUpgrade(r) {
		handlers.ProxyWebSocket(w, r, targetURL)
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}

	for k, vv := range r.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	for _, v := range r.Header["Upgrade"] {
		if strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
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
