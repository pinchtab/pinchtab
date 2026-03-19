package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestAuthAPIHandleLogin(t *testing.T) {
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.HandleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != authn.CookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, authn.CookieName)
	}
	if cookie.Value == "secret-token" {
		t.Fatal("expected opaque session cookie, not raw bearer token")
	}
	if !cookie.HttpOnly {
		t.Fatal("expected auth cookie to be HttpOnly")
	}
	if !cookie.Secure {
		t.Fatal("expected auth cookie to be Secure")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie SameSite = %v, want %v", cookie.SameSite, http.SameSiteStrictMode)
	}
	if !sessions.Validate(cookie.Value, "secret-token") {
		t.Fatal("expected session cookie value to validate against current token")
	}
}

func TestAuthAPIHandleLoginRejectsBadToken(t *testing.T) {
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, authn.NewSessionManager(authn.SessionConfig{}))

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != authn.CookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("expected expired auth cookie on failure, got %+v", cookies)
	}
}

func TestAuthAPIHandleLogoutClearsCookie(t *testing.T) {
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()

	api.HandleLogout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != authn.CookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("expected expired auth cookie, got %+v", cookies)
	}
	if !cookies[0].Secure {
		t.Fatal("expected expired auth cookie to remain Secure")
	}
	if sessions.Validate(sessionID, "secret-token") {
		t.Fatal("expected logout to revoke session")
	}
}

func TestAuthAPIHandleElevateMarksSessionElevated(t *testing.T) {
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "/api/auth/elevate", strings.NewReader(`{"token":"secret-token"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()

	api.HandleElevate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !sessions.IsElevated(sessionID, "secret-token") {
		t.Fatal("expected session to be elevated after re-entering the token")
	}
}

func TestAuthAPIHandleElevateRejectsBadToken(t *testing.T) {
	sessions := authn.NewSessionManager(authn.SessionConfig{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "/api/auth/elevate", strings.NewReader(`{"token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()

	api.HandleElevate(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if sessions.IsElevated(sessionID, "secret-token") {
		t.Fatal("expected session to remain unelevated after bad token")
	}
}

func TestAuthAPIHandleLoginRateLimitsRepeatedFailures(t *testing.T) {
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, authn.NewSessionManager(authn.SessionConfig{}))
	api.loginLimiter = authn.NewAttemptLimiter(authn.AttemptLimiterConfig{
		Window:      time.Minute,
		MaxAttempts: 1,
	})

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("first status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	api.HandleLogin(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("expected Retry-After header when login is rate limited")
	}
}

func TestAuthAPIHandleLoginRateLimitIgnoresSpoofedForwardedHeaders(t *testing.T) {
	api := NewAuthAPI(&config.RuntimeConfig{Token: "secret-token"}, authn.NewSessionManager(authn.SessionConfig{}))
	api.loginLimiter = authn.NewAttemptLimiter(authn.AttemptLimiterConfig{
		Window:      time.Minute,
		MaxAttempts: 1,
	})

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`))
	req.RemoteAddr = "198.51.100.10:41000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	w := httptest.NewRecorder()
	api.HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("first status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`))
	req.RemoteAddr = "198.51.100.10:41001"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.2")
	req.Header.Set("X-Real-Ip", "203.0.113.3")
	w = httptest.NewRecorder()
	api.HandleLogin(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}
