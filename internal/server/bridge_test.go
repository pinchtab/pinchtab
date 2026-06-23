package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
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
