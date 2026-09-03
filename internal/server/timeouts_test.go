package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func TestServerTimeoutBudgets(t *testing.T) {
	if serverReadHeaderTimeout >= serverReadTimeout {
		t.Errorf("ReadHeaderTimeout (%v) should be less than ReadTimeout (%v)", serverReadHeaderTimeout, serverReadTimeout)
	}
	if serverReadTimeout >= serverWriteTimeout {
		t.Errorf("ReadTimeout (%v) should be less than WriteTimeout (%v)", serverReadTimeout, serverWriteTimeout)
	}
	if serverWriteTimeout < httpx.MaxNavigationHTTPDuration {
		t.Errorf("WriteTimeout (%v) cuts off the navigation HTTP budget (%v)", serverWriteTimeout, httpx.MaxNavigationHTTPDuration)
	}
	if serverIdleTimeout <= 0 {
		t.Errorf("IdleTimeout must be positive, got %v", serverIdleTimeout)
	}
}

func TestDashboardHandlerChainAppliesRateLimit(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret"}
	sessions := browsersession.NewManager(browsersession.Config{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /protected", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := handlers.RequestIDMiddleware(
		activity.Middleware(
			nil,
			"server",
			handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.CorsMiddleware(config.NewLive(cfg), handlers.AuthMiddlewareWithSessions(config.NewLive(cfg), sessions, nil, mux)))),
		),
	)

	for i := 0; i < 3000; i++ {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.RemoteAddr = "198.51.100.11:41001"
		req.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.RemoteAddr = "198.51.100.11:41001"
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit exceeded, got %d", w.Code)
	}
}
