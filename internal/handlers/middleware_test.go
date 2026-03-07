package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/web"
)

func TestAuthMiddleware_NoToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: ""}

	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should have been called")
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}

	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should have been called with valid token")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}

	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT have been called with invalid token")
	}
	if w.Code != 401 {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestAuthMiddleware_MissingTokenHeader(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}

	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestAuthMiddleware_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		authHeader string
		wantCode   int
		wantCalled bool
	}{
		{"correct token", "secret", "Bearer secret", 200, true},
		{"wrong token", "secret", "Bearer wrong", 401, false},
		{"partial match", "secret", "Bearer secre", 401, false},
		{"empty bearer", "secret", "Bearer ", 401, false},
		{"missing header", "secret", "", 401, false},
		{"no token configured", "", "", 200, true},
		{"no token configured with header", "", "Bearer anything", 200, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{Token: tt.token}
			called := false
			handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(200)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestCorsMiddleware(t *testing.T) {
	handler := CorsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("OPTIONS expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}

	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("GET expected 200, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header on GET")
	}
}

func TestLoggingMiddleware(t *testing.T) {
	resetObservabilityForTests()
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestLoggingMiddleware_RecordsFailure(t *testing.T) {
	resetObservabilityForTests()
	handler := RequestIDMiddleware(LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})))

	req := httptest.NewRequest("GET", "/boom", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	snap := FailureSnapshot()
	if got := snap["requestsFailed"].(uint64); got != 1 {
		t.Fatalf("requestsFailed = %d, want 1", got)
	}
	recent, ok := snap["recent"].([]FailureEvent)
	if !ok || len(recent) != 1 {
		t.Fatalf("recent failures = %#v, want 1 event", snap["recent"])
	}
	if recent[0].Path != "/boom" {
		t.Fatalf("recent path = %q, want /boom", recent[0].Path)
	}
	if recent[0].RequestID == "" {
		t.Fatal("expected request id on failure event")
	}
}

func TestRequestIDMiddleware_SetsHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected X-Request-Id")
	}
}

func TestRateLimitMiddleware_AllowsRequest(t *testing.T) {
	handler := RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_BypassHealthAndMetrics(t *testing.T) {
	handler := RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	for _, p := range []string{"/health", "/metrics"} {
		req := httptest.NewRequest("GET", p, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for %s, got %d", p, w.Code)
		}
	}
}

func TestEvictStaleRateBuckets_DeletesEmptyHosts(t *testing.T) {
	now := time.Now()
	window := 10 * time.Second

	rateMu.Lock()
	rateBuckets = map[string][]time.Time{
		"stale-only": {now.Add(-2 * window)},
		"mixed":      {now.Add(-2 * window), now.Add(-window / 2)},
		"fresh":      {now.Add(-window / 3)},
	}
	rateMu.Unlock()

	evictStaleRateBuckets(now, window)

	rateMu.Lock()
	defer rateMu.Unlock()

	if _, ok := rateBuckets["stale-only"]; ok {
		t.Fatal("expected stale-only bucket to be deleted")
	}
	if got := len(rateBuckets["mixed"]); got != 1 {
		t.Fatalf("expected mixed bucket to keep 1 hit, got %d", got)
	}
	if got := len(rateBuckets["fresh"]); got != 1 {
		t.Fatalf("expected fresh bucket to keep 1 hit, got %d", got)
	}

	// Cleanup
	rateBuckets = map[string][]time.Time{}
}

func TestStatusWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &web.StatusWriter{ResponseWriter: w, Code: 200}

	sw.WriteHeader(404)
	if sw.Code != 404 {
		t.Errorf("expected 404, got %d", sw.Code)
	}
}
