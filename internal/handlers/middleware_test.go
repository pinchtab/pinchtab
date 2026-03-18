package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
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

	if called {
		t.Error("handler should NOT have been called when token is missing")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when token is missing, got %d", w.Code)
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

func TestAuthMiddleware_ValidCookie(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Referer", "http://example.com/dashboard")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should have been called with valid cookie auth")
	}
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieRestrictedEndpointRejected(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodPost, "/evaluate", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not allow cookie auth on restricted endpoint")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieCrossOriginRejected(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not allow cross-origin cookie auth")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieRequestWithoutOriginOrRefererRejected(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not allow cookie auth without same-origin headers")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieSameOriginRefererAccepted(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Referer", "http://example.com/dashboard")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should allow same-origin cookie-authenticated request with referer")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieIgnoresForwardedOriginHints(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "127.0.0.1:9867"
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Origin", "https://pinchtab.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pinchtab.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should ignore forwarded host/proto headers for cookie same-origin checks")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieWebSocketRequiresSameOrigin(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/instances/inst1/proxy/screencast", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not allow cookie-authenticated websocket upgrade without an Origin header")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	called = false
	req = httptest.NewRequest(http.MethodGet, "/instances/inst1/proxy/screencast", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Origin", "http://example.com")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should allow same-origin cookie-authenticated websocket upgrade")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_CookieElevatedEndpointRequiresElevation(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	called := false
	handler := AuthMiddlewareWithSessions(cfg, sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Referer", "http://example.com/dashboard")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not allow elevated endpoint without elevation")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	if !sessions.Elevate(sessionID, cfg.Token) {
		t.Fatal("expected session elevation to succeed in test setup")
	}

	called = false
	req = httptest.NewRequest(http.MethodPut, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	req.Header.Set("Referer", "http://example.com/dashboard")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should allow elevated endpoint with elevation")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_HeaderAllowsRestrictedEndpoint(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodPost, "/evaluate", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should allow header auth on restricted endpoint")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_QueryTokenRejected(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}

	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test?token=secret123", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not accept query-string tokens")
	}
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_PublicDashboardPathBypassesAuth(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should have been called for public dashboard path")
	}
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_PublicDashboardSubpathBypassesAuth(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/dashboard/monitoring", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should have been called for public dashboard subpath")
	}
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_PublicAuthPathBypassesAuth(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	called := false
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler should have been called for public auth path")
	}
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_ProtectedAPIStillRequiresAuth(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret123"}
	handler := AuthMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
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
		{"query token rejected", "secret", "", 401, false},
		{"no token configured", "", "", http.StatusServiceUnavailable, false},
		{"no token configured with header", "", "Bearer anything", http.StatusServiceUnavailable, false},
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
			if tt.name == "query token rejected" {
				req = httptest.NewRequest("GET", "/test?token=secret", nil)
			}
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
	handler := CorsMiddleware(&config.RuntimeConfig{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("OPTIONS expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("GET expected 200, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header on GET")
	}
}

func TestCorsMiddleware_AuthEnabledAllowsOnlySameOrigin(t *testing.T) {
	handler := CorsMiddleware(&config.RuntimeConfig{Token: "secret"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Host = "pinchtab.test"
	req.Header.Set("Origin", "http://pinchtab.test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Fatalf("OPTIONS expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://pinchtab.test" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want same origin", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCorsMiddleware_AuthEnabledRejectsCrossOriginPreflight(t *testing.T) {
	handler := CorsMiddleware(&config.RuntimeConfig{Token: "secret"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Host = "pinchtab.test"
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("OPTIONS expected 403, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
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
	sw := &httpx.StatusWriter{ResponseWriter: w, Code: 200}

	sw.WriteHeader(404)
	if sw.Code != 404 {
		t.Errorf("expected 404, got %d", sw.Code)
	}
}
