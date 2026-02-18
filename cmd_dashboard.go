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
)

// runDashboard starts a lightweight dashboard server — no Chrome, no bridge.
// It manages Pinchtab instances via the orchestrator and serves the dashboard UI.
func runDashboard() {
	dashPort := port // reuse --port / BRIDGE_PORT
	if dashPort == "" {
		dashPort = "9870"
	}

	slog.Info("🦀 Pinchtab Dashboard", "port", dashPort)

	profilesDir := filepath.Join(stateDir, "profiles")
	os.MkdirAll(profilesDir, 0755)

	profMgr := NewProfileManager(profilesDir)
	dashboard := NewDashboard(nil)
	orchestrator := NewOrchestrator(profilesDir)

	mux := http.NewServeMux()

	// Dashboard UI + SSE
	dashboard.RegisterHandlers(mux)
	orchestrator.RegisterHandlers(mux)
	profMgr.RegisterHandlers(mux)

	// Health endpoint for the dashboard itself
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]string{"status": "ok", "mode": "dashboard"})
	})

	// Proxy common bridge endpoints to the default instance
	// This lets the dashboard UI call /tabs, /screencast/tabs etc on the dashboard port
	proxyEndpoints := []string{
		"/tabs", "/snapshot", "/screenshot", "/text",
		"/navigate", "/action", "/actions", "/evaluate",
		"/tab", "/tab/lock", "/tab/unlock",
		"/cookies", "/stealth/status", "/fingerprint/rotate",
		"/screencast", "/screencast/tabs",
	}
	for _, ep := range proxyEndpoints {
		endpoint := ep // capture
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			// Find first running instance and proxy to it
			instances := orchestrator.List()
			var target string
			for _, inst := range instances {
				if inst.Status == "running" {
					target = inst.URL
					break
				}
			}
			if target == "" {
				jsonErr(w, 503, fmt.Errorf("no running instances — launch one from the Profiles tab"))
				return
			}
			proxyRequest(w, r, target+endpoint)
		})
	}

	// Profile observer
	profileObserver := func(evt AgentEvent) {
		if evt.Profile != "" {
			profMgr.tracker.Record(evt.Profile, ActionRecord{
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

	handler := dashboard.TrackingMiddleware(
		[]EventObserver{profileObserver},
		loggingMiddleware(corsMiddleware(authMiddleware(mux))),
	)

	srv := &http.Server{Addr: ":" + dashPort, Handler: handler}

	// Auto-launch default instance if configured
	defaultProfile := os.Getenv("PINCHTAB_DEFAULT_PROFILE")
	defaultPort := os.Getenv("PINCHTAB_DEFAULT_PORT")
	if defaultPort == "" {
		defaultPort = "9867"
	}
	if defaultProfile == "" {
		defaultProfile = "default"
	}

	// Ensure default profile exists
	os.MkdirAll(filepath.Join(profilesDir, defaultProfile, "Default"), 0755)

	autoLaunch := os.Getenv("PINCHTAB_NO_AUTO_LAUNCH") == ""
	if autoLaunch {
		go func() {
			// Wait for server to be ready
			time.Sleep(500 * time.Millisecond)
			headlessDefault := os.Getenv("PINCHTAB_HEADED") == ""
			inst, err := orchestrator.Launch(defaultProfile, defaultPort, headlessDefault)
			if err != nil {
				slog.Warn("auto-launch failed", "err", err)
				return
			}
			slog.Info("auto-launched default instance", "id", inst.ID, "port", defaultPort, "headless", headlessDefault)
		}()
	}

	// Shutdown
	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down dashboard...")
			dashboard.Shutdown()
			orchestrator.Shutdown()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			srv.Shutdown(ctx)
		})
	}

	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]string{"status": "shutting down"})
		go doShutdown()
	})

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		doShutdown()
	}()

	slog.Info("dashboard ready", "url", fmt.Sprintf("http://localhost:%s/dashboard", dashPort))

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

// proxyRequest forwards an HTTP request to a target URL.
// For WebSocket upgrades (screencast), it does a WebSocket proxy.
func proxyRequest(w http.ResponseWriter, r *http.Request, targetURL string) {
	// Preserve query params
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Check if this is a WebSocket upgrade (screencast)
	if isWebSocketUpgrade(r) {
		proxyWebSocket(w, r, targetURL)
		return
	}

	// Regular HTTP proxy
	client := &http.Client{Timeout: 30 * time.Second}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}

	// Forward headers
	for k, vv := range r.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer resp.Body.Close()

	// Copy response
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
			w.Write(buf[:n])
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
