package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestAuthAPIHandleLogin(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "https://pinchtab.example/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
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

func TestAuthAPIHandleLogin_LocalhostHTTPUsesNonSecureCookie(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "http://localhost:9867/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
	req.Host = "localhost:9867"
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
	if cookies[0].Secure {
		t.Fatal("expected localhost http auth cookie to omit Secure so browser sessions work reliably")
	}
}

func TestAuthAPIHandleLogin_LANHTTPUsesNonSecureCookie(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "http://192.168.1.50:9867/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
	req.Host = "192.168.1.50:9867"
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
	if cookies[0].Secure {
		t.Fatal("expected LAN http auth cookie to omit Secure so browser sessions work reliably")
	}
}

func TestAuthAPIHandleLogin_TrustedProxyHTTPSUsesSecureCookie(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	api := newAuthAPIForTest(&config.RuntimeConfig{
		Token:             "secret-token",
		TrustProxyHeaders: true,
	}, sessions)

	req := httptest.NewRequest("POST", "http://127.0.0.1:9867/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
	req.Host = "127.0.0.1:9867"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pinchtab.example")
	w := httptest.NewRecorder()

	api.HandleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if !cookies[0].Secure {
		t.Fatal("expected proxied https auth cookie to remain Secure")
	}
}

func TestAuthAPIHandleLogin_CookieSecureFalseOverridesHTTPS(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	forceInsecure := false
	api := newAuthAPIForTest(&config.RuntimeConfig{
		Token:        "secret-token",
		CookieSecure: &forceInsecure,
	}, sessions)

	req := httptest.NewRequest("POST", "https://pinchtab.example/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
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
	if cookies[0].Secure {
		t.Fatal("expected explicit cookieSecure=false to disable Secure even on https request")
	}
}

func TestAuthAPIHandleLogin_CookieSecureTrueRejectsPlainHTTP(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	forceSecure := true
	api := newAuthAPIForTest(&config.RuntimeConfig{
		Token:        "secret-token",
		CookieSecure: &forceSecure,
	}, sessions)

	req := httptest.NewRequest("POST", "http://192.168.1.50:9867/api/auth/login", strings.NewReader(`{"token":"secret-token"}`))
	req.Host = "192.168.1.50:9867"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.HandleLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "secure_cookie_requires_https") {
		t.Fatalf("response = %q, want secure_cookie_requires_https error", w.Body.String())
	}
	if cookies := w.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("cookies = %+v, want none", cookies)
	}
}

func TestAuthAPIHandleLoginRejectsBadToken(t *testing.T) {
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, browsersession.NewManager(browsersession.Config{}))

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
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "https://pinchtab.example/api/auth/logout", nil)
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

func TestAuthAPIHandleLogout_LocalhostHTTPClearsNonSecureCookie(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "http://localhost:9867/api/auth/logout", nil)
	req.Host = "localhost:9867"
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
	if cookies[0].Secure {
		t.Fatal("expected localhost http logout cookie clearing to omit Secure")
	}
}

func TestAuthAPIHandleLogout_LANHTTPClearsNonSecureCookie(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

	req := httptest.NewRequest("POST", "http://192.168.1.50:9867/api/auth/logout", nil)
	req.Host = "192.168.1.50:9867"
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
	if cookies[0].Secure {
		t.Fatal("expected LAN http logout cookie clearing to omit Secure")
	}
}

func TestAuthAPIHandleElevateMarksSessionElevated(t *testing.T) {
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

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
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, sessions)

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
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, browsersession.NewManager(browsersession.Config{}))
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

// The sibling of the spoofed-headers test, on the other side of the flag: behind a
// trusted proxy every request carries the proxy's peer address, so bucketing by it
// lets one client failing its attempts lock every dashboard user out.
func TestAuthAPIHandleLoginRateLimitsEachForwardedClientSeparatelyWhenProxyIsTrusted(t *testing.T) {
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token", TrustProxyHeaders: true}, browsersession.NewManager(browsersession.Config{}))
	api.loginLimiter = authn.NewAttemptLimiter(authn.AttemptLimiterConfig{
		Window:      time.Minute,
		MaxAttempts: 1,
	})

	login := func(forwardedFor string) int {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"token":"wrong"}`))
		req.RemoteAddr = "198.51.100.10:41000"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", forwardedFor)
		req = req.WithContext(authn.WithClientIP(req.Context(), authn.ResolveClientIP(req, true)))
		w := httptest.NewRecorder()
		api.HandleLogin(w, req)
		return w.Code
	}

	if code := login("203.0.113.1"); code != http.StatusUnauthorized {
		t.Fatalf("first client status = %d, want %d", code, http.StatusUnauthorized)
	}
	if code := login("203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("same client repeating status = %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := login("203.0.113.2"); code != http.StatusUnauthorized {
		t.Fatalf("second client status = %d, want %d; one client's failures must not lock out everyone behind the proxy", code, http.StatusUnauthorized)
	}
}

func TestAuthAPIHandleLoginRateLimitIgnoresSpoofedForwardedHeaders(t *testing.T) {
	api := newAuthAPIForTest(&config.RuntimeConfig{Token: "secret-token"}, browsersession.NewManager(browsersession.Config{}))
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
