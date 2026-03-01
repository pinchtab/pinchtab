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

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/profiles"
	"github.com/pinchtab/pinchtab/internal/web"
)

// runDashboard starts a lightweight dashboard server — no Chrome, no bridge.
// It manages Pinchtab instances via the orchestrator and serves the dashboard UI.
func runDashboard(cfg *config.RuntimeConfig) {
	dashPort := cfg.Port
	if dashPort == "" {
		dashPort = "9870"
	}

	slog.Info("🦀 Pinchtab Dashboard", "port", dashPort)

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

	mux := http.NewServeMux()

	dash.RegisterHandlers(mux)
	orch.RegisterHandlers(mux)
	profMgr.RegisterHandlers(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		web.JSON(w, 200, map[string]string{"status": "ok", "mode": "dashboard"})
	})

	// Special handler for /tabs - return empty list if no instances
	mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
		target := orch.FirstRunningURL()
		if target == "" {
			// No instances running, return empty tabs list
			web.JSON(w, 200, map[string]interface{}{"tabs": []interface{}{}})
			return
		}
		proxyRequest(w, r, target+"/tabs")
	})

	proxyEndpoints := []string{
		"GET /snapshot", "GET /screenshot", "GET /text",
		"GET /tabs/{tabId}/pdf", "POST /tabs/{tabId}/pdf",
		"POST /navigate", "POST /action", "POST /actions", "POST /evaluate",
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
			// Extract path from endpoint (remove method prefix)
			path := r.URL.Path
			proxyRequest(w, r, target+path)
		})
	}

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
		Addr:              cfg.Bind + ":" + dashPort,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	autoLaunch := strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "1") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "true") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "yes")
	if autoLaunch {
		defaultProfile := os.Getenv("PINCHTAB_DEFAULT_PROFILE")
		defaultPort := os.Getenv("PINCHTAB_DEFAULT_PORT")
		// defaultPort is optional - will auto-allocate if not specified
		if defaultProfile == "" {
			defaultProfile = "default"
		}

		if err := os.MkdirAll(filepath.Join(profilesDir, defaultProfile, "Default"), 0755); err != nil {
			slog.Warn("failed to create default profile dir", "err", err)
		}

		go func() {
			time.Sleep(500 * time.Millisecond)
			headlessDefault := os.Getenv("PINCHTAB_HEADED") == ""
			inst, err := orch.Launch(defaultProfile, defaultPort, headlessDefault)
			if err != nil {
				slog.Warn("auto-launch failed", "err", err)
				return
			}
			slog.Info("auto-launched default instance", "id", inst.ID, "port", inst.Port, "headless", headlessDefault)
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

	slog.Info("dashboard ready", "url", fmt.Sprintf("http://localhost:%s/dashboard", dashPort))

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
