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

	profMgr := NewProfileManager(profilesDir)
	dashboard := NewDashboard(nil)
	orchestrator := NewOrchestrator(profilesDir)
	orchestrator.SetProfileManager(profMgr)

	mux := http.NewServeMux()

	dashboard.RegisterHandlers(mux)
	orchestrator.RegisterHandlers(mux)
	profMgr.RegisterHandlers(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]string{"status": "ok", "mode": "dashboard"})
	})

	proxyEndpoints := []string{
		"/tabs", "/snapshot", "/screenshot", "/text",
		"/navigate", "/action", "/actions", "/evaluate",
		"/tab", "/tab/lock", "/tab/unlock",
		"/cookies", "/stealth/status", "/fingerprint/rotate",
		"/screencast", "/screencast/tabs",
	}
	for _, ep := range proxyEndpoints {
		endpoint := ep
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			target := orchestrator.FirstRunningURL()
			if target == "" {
				jsonErr(w, 503, fmt.Errorf("no running instances — launch one from the Profiles tab"))
				return
			}
			proxyRequest(w, r, target+endpoint)
		})
	}

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

	handler := dashboard.TrackingMiddleware(
		[]EventObserver{profileObserver},
		loggingMiddleware(corsMiddleware(authMiddleware(mux))),
	)

	srv := &http.Server{Addr: ":" + dashPort, Handler: handler}

	autoLaunch := strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "1") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "true") ||
		strings.EqualFold(os.Getenv("PINCHTAB_AUTO_LAUNCH"), "yes")
	if autoLaunch {
		defaultProfile := os.Getenv("PINCHTAB_DEFAULT_PROFILE")
		defaultPort := os.Getenv("PINCHTAB_DEFAULT_PORT")
		if defaultPort == "" {
			defaultPort = "9867"
		}
		if defaultProfile == "" {
			defaultProfile = "default"
		}

		if err := os.MkdirAll(filepath.Join(profilesDir, defaultProfile, "Default"), 0755); err != nil {
			slog.Warn("failed to create default profile dir", "err", err)
		}

		go func() {

			time.Sleep(500 * time.Millisecond)
			headlessDefault := os.Getenv("PINCHTAB_HEADED") == ""
			inst, err := orchestrator.Launch(defaultProfile, defaultPort, headlessDefault)
			if err != nil {
				slog.Warn("auto-launch failed", "err", err)
				return
			}
			slog.Info("auto-launched default instance", "id", inst.ID, "port", defaultPort, "headless", headlessDefault)
		}()
	} else {
		slog.Info("dashboard auto-launch disabled", "hint", "set PINCHTAB_AUTO_LAUNCH=1 to enable")
	}

	shutdownOnce := &sync.Once{}
	doShutdown := func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down dashboard...")
			dashboard.Shutdown()
			orchestrator.Shutdown()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				slog.Error("shutdown http", "err", err)
			}
		})
	}

	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]string{"status": "shutting down"})
		go doShutdown()
	})

	go func() {
		sig := make(chan os.Signal, 2)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		go doShutdown()
		<-sig
		slog.Warn("force shutdown requested")
		orchestrator.ForceShutdown()
		os.Exit(130)
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

	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	if isWebSocketUpgrade(r) {
		proxyWebSocket(w, r, targetURL)
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}

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
