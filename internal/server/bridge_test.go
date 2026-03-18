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
		name       string
		engine     string
		wantRouter bool
	}{
		{name: "chrome", engine: "chrome", wantRouter: false},
		{name: "lite", engine: "lite", wantRouter: true},
		{name: "auto", engine: "auto", wantRouter: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handlers.New(nil, &config.RuntimeConfig{Engine: tt.engine}, nil, nil, nil)
			configureBridgeRouter(h, &config.RuntimeConfig{Engine: tt.engine})
			if (h.Router != nil) != tt.wantRouter {
				t.Fatalf("router presence = %v, want %v", h.Router != nil, tt.wantRouter)
			}
			if h.Router != nil && string(h.Router.Mode()) != tt.engine {
				t.Fatalf("router mode = %q, want %q", h.Router.Mode(), tt.engine)
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

	for i := 0; i < 120; i++ {
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
