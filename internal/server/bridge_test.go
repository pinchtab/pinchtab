package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.AuthMiddleware(cfg, mux))),
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
