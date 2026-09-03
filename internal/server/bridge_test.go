package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridgeregistry"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

func TestConfigureBridgeRouter(t *testing.T) {
	tests := []struct {
		name            string
		browsersDefault string
		wantWrapped     bool // true if Bridge should be replaced with BridgeAdapter
	}{
		{name: "chrome", browsersDefault: "chrome", wantWrapped: false},
		{name: "cloak", browsersDefault: "cloak", wantWrapped: false},
		{name: "ghost-chrome", browsersDefault: "ghost-chrome", wantWrapped: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{DefaultBrowser: tt.browsersDefault}
			h := handlers.New(nil, cfg, nil, nil, nil)
			origBridge := h.Bridge
			configureBridgeRouter(h, cfg)
			wasWrapped := h.Bridge != origBridge
			if wasWrapped != tt.wantWrapped {
				t.Fatalf("Bridge wrapped = %v, want %v", wasWrapped, tt.wantWrapped)
			}
		})
	}
}

func TestRegisterBridgeMapsRuntimeIdentityAndCleansUp(t *testing.T) {
	cfg := &config.RuntimeConfig{
		StateDir:          t.TempDir(),
		Bind:              "127.0.0.1",
		Port:              "9878",
		CDPAttachURL:      "ws://user:password@127.0.0.1:9222/devtools/browser/browser-guid?token=secret",
		DefaultBrowser:    "cloak",
		RemoteBrowserName: "work-profile",
	}
	registration, err := registerBridge(cfg, &net.TCPAddr{IP: net.ParseIP(cfg.Bind), Port: 9878})
	if err != nil {
		t.Fatalf("registerBridge() error = %v", err)
	}
	states, err := bridgeregistry.List(cfg.StateDir, false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("List() returned %d records, want 1", len(states))
	}
	got := states[0]
	if got.Address != cfg.Bind || got.Port != cfg.Port || got.BrowserType != "cloak" || got.BrowserLabel != "work-profile" {
		t.Fatalf("registered bridge = %+v", got)
	}
	if got.CDPIdentity == "" || strings.Contains(got.CDPIdentity, "password") || strings.Contains(got.CDPIdentity, "browser-guid") {
		t.Fatalf("unsafe CDP identity %q", got.CDPIdentity)
	}
	if err := registration.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	states, err = bridgeregistry.List(cfg.StateDir, false)
	if err != nil || len(states) != 0 {
		t.Fatalf("records after cleanup = %+v, err %v", states, err)
	}
}

func TestRegisterBridgeUsesBoundEphemeralPort(t *testing.T) {
	cfg := &config.RuntimeConfig{
		StateDir:       t.TempDir(),
		Bind:           "127.0.0.1",
		Port:           "0",
		DefaultBrowser: config.BrowserChrome,
	}
	registration, err := registerBridge(cfg, &net.TCPAddr{IP: net.ParseIP(cfg.Bind), Port: 43210})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = registration.Close() }()
	states, err := bridgeregistry.List(cfg.StateDir, false)
	if err != nil || len(states) != 1 {
		t.Fatalf("registered states = %+v, error = %v", states, err)
	}
	if states[0].Port != "43210" {
		t.Fatalf("registered port = %q, want bound port 43210", states[0].Port)
	}
}

func TestApplyBoundBridgePortUpdatesRuntimeConfig(t *testing.T) {
	cfg := &config.RuntimeConfig{Port: "0"}
	applyBoundBridgePort(cfg, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 43210})
	if cfg.Port != "43210" {
		t.Fatalf("runtime port = %q, want bound port 43210", cfg.Port)
	}
	applyBoundBridgePort(nil, &net.TCPAddr{Port: 1})
	applyBoundBridgePort(cfg, nil)
}

func TestBridgeHandlerChainAppliesRateLimit(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /protected", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := handlers.RequestIDMiddleware(
		activity.Middleware(
			nil,
			"bridge",
			handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.AuthMiddleware(config.NewLive(cfg), mux))),
		),
	)

	for i := 0; i < 3000; i++ {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.RemoteAddr = "198.51.100.10:41000"
		req.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.RemoteAddr = "198.51.100.10:41000"
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit exceeded, got %d", w.Code)
	}
}

// freeTCPPort returns a currently-unused loopback port by binding then
// immediately releasing it. Small bind race accepted — the standard pattern
// for picking a port a test-owned server will rebind moments later.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("release free port: %v", err)
	}
	return port
}

// bridgeListenerReachable dials the port directly rather than calling
// GET /health: the health handler lazily auto-starts a real browser on first
// call, which is unrelated to (and much heavier/flakier than) what this test
// checks — whether the HTTP listener itself is still up.
func bridgeListenerReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// TestRunBridgeServerShutdownStopsListening is a regression test for a bug
// where POST /shutdown on a bridge-mode server never actually stopped the
// HTTP listener: the handler only ran bridgeInstance.Cleanup(), while
// server.Shutdown was wired solely into the SIGINT/SIGTERM select loop that
// an HTTP-triggered shutdown never reaches. The listener stayed reachable
// forever, which is exactly what broke the orchestrator E2E test asserting a
// registered bridge instance actually stops.
func TestRunBridgeServerShutdownStopsListening(t *testing.T) {
	port := freeTCPPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg := &config.RuntimeConfig{
		Bind:           "127.0.0.1",
		Port:           fmt.Sprintf("%d", port),
		Token:          "test-shutdown-token",
		StateDir:       t.TempDir(),
		DefaultBrowser: config.BrowserChrome,
		ActionTimeout:  time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunBridgeServer(cfg, "test")
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !bridgeListenerReachable(addr) {
		if time.Now().After(deadline) {
			t.Fatal("bridge server never became reachable")
		}
		time.Sleep(20 * time.Millisecond)
	}

	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/shutdown", nil)
	if err != nil {
		t.Fatalf("build shutdown request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /shutdown: %v", err)
	}
	_ = resp.Body.Close()

	deadline = time.Now().Add(5 * time.Second)
	for bridgeListenerReachable(addr) {
		if time.Now().After(deadline) {
			t.Fatal("bridge listener stayed reachable after /shutdown, want it to stop")
		}
		time.Sleep(20 * time.Millisecond)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunBridgeServer did not return after /shutdown")
	}
}
