package dashboard

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type AuthAPI struct {
	runtime      *config.RuntimeConfig
	sessions     *browsersession.Manager
	loginLimiter *authn.AttemptLimiter
}

func NewAuthAPI(runtime *config.RuntimeConfig, sessions *browsersession.Manager) *AuthAPI {
	return &AuthAPI{
		runtime:  runtime,
		sessions: sessions,
		loginLimiter: authn.NewAttemptLimiter(authn.AttemptLimiterConfig{
			Window:      authn.DefaultLoginRateLimitWindow,
			MaxAttempts: authn.DefaultLoginRateLimitMaxAttempt,
		}),
	}
}

func (a *AuthAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", a.HandleLogin)
	mux.HandleFunc("POST /api/auth/elevate", a.HandleElevate)
	mux.HandleFunc("POST /api/auth/logout", a.HandleLogout)
}

func (a *AuthAPI) HandleLogin(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(a.runtime.Token)
	if token == "" {
		httpx.ErrorCode(w, http.StatusServiceUnavailable, "token_required", "server token is not configured", false, nil)
		return
	}
	clientIP := authn.ClientIP(r)
	if a.loginLimiter != nil {
		if allowed, retryAfter := a.loginLimiter.Allow(clientIP); !allowed {
			retryAfterSec := secondsCeil(retryAfter)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
			authn.AuditWarn(r, "auth.login_rate_limited",
				"retryAfterSec", retryAfterSec,
				"windowSec", int(a.loginLimiter.Window().Seconds()),
				"maxAttempts", a.loginLimiter.MaxAttempts(),
			)
			httpx.ErrorCode(w, http.StatusTooManyRequests, "login_rate_limited", "too many login attempts", true, map[string]any{
				"retryAfterSec": retryAfterSec,
				"windowSec":     int(a.loginLimiter.Window().Seconds()),
				"maxAttempts":   a.loginLimiter.MaxAttempts(),
			})
			return
		}
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_auth_json", "invalid auth payload", false, nil)
		return
	}

	provided := strings.TrimSpace(req.Token)
	if provided == "" {
		if a.loginLimiter != nil {
			a.loginLimiter.RecordFailure(clientIP)
		}
		authn.AuditWarn(r, "auth.login_failed", "reason", "missing_token")
		w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="missing_token"`)
		httpx.ErrorCode(w, http.StatusUnauthorized, "missing_token", "unauthorized", false, nil)
		return
	}

	if a.requiresHTTPSForDashboardSession(r) {
		httpx.ErrorCode(w, http.StatusBadRequest, "secure_cookie_requires_https", "server.cookieSecure=true requires HTTPS for dashboard login", false, map[string]any{
			"hint":   "Use HTTPS directly or through a trusted reverse proxy, or set server.cookieSecure back to auto/false for plain HTTP local use.",
			"remedy": "If TLS terminates in front of PinchTab, also enable server.trustProxyHeaders so forwarded https requests are recognized.",
		})
		return
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		if a.loginLimiter != nil {
			a.loginLimiter.RecordFailure(clientIP)
		}
		authn.ClearSessionCookie(w, r, a.runtime != nil && a.runtime.TrustProxyHeaders, cookieSecureSetting(a.runtime))
		authn.AuditWarn(r, "auth.login_failed", "reason", "bad_token")
		w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
		httpx.ErrorCode(w, http.StatusUnauthorized, "bad_token", "unauthorized", false, nil)
		return
	}

	if a.sessions == nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "session_unavailable", "dashboard sessions are not configured", false, nil)
		return
	}
	if creds := authn.CredentialsFromRequest(r); creds.Method == authn.MethodCookie {
		a.sessions.Revoke(creds.Value)
		authn.AuditLog(r, "auth.session_revoked", "reason", "replaced")
	}
	if a.loginLimiter != nil {
		a.loginLimiter.Reset(clientIP)
	}
	sessionID, err := a.sessions.Create(token)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err)
		return
	}
	authn.SetSessionCookie(w, r, sessionID, a.sessions.MaxLifetime(), a.runtime != nil && a.runtime.TrustProxyHeaders, cookieSecureSetting(a.runtime))
	authn.AuditLog(r, "auth.session_created",
		"sessionIdleSec", int(a.sessions.IdleTimeout().Seconds()),
		"sessionMaxLifetimeSec", int(a.sessions.MaxLifetime().Seconds()),
	)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *AuthAPI) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if a.sessions != nil {
		if creds := authn.CredentialsFromRequest(r); creds.Method == authn.MethodCookie {
			a.sessions.Revoke(creds.Value)
			authn.AuditLog(r, "auth.session_revoked", "reason", "logout")
		}
	}
	authn.ClearSessionCookie(w, r, a.runtime != nil && a.runtime.TrustProxyHeaders, cookieSecureSetting(a.runtime))
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *AuthAPI) HandleElevate(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(a.runtime.Token)
	if token == "" {
		httpx.ErrorCode(w, http.StatusServiceUnavailable, "token_required", "server token is not configured", false, nil)
		return
	}
	if a.sessions == nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "session_unavailable", "dashboard sessions are not configured", false, nil)
		return
	}

	creds := authn.CredentialsFromRequest(r)
	if creds.Method != authn.MethodCookie || creds.Value == "" {
		httpx.ErrorCode(w, http.StatusForbidden, "session_auth_required", "dashboard session required", false, nil)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_auth_json", "invalid auth payload", false, nil)
		return
	}

	provided := strings.TrimSpace(req.Token)
	if provided == "" {
		authn.AuditWarn(r, "auth.elevation_failed", "reason", "missing_token")
		w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="missing_token"`)
		httpx.ErrorCode(w, http.StatusUnauthorized, "missing_token", "unauthorized", false, nil)
		return
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		authn.AuditWarn(r, "auth.elevation_failed", "reason", "bad_token")
		w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
		httpx.ErrorCode(w, http.StatusUnauthorized, "bad_token", "unauthorized", false, nil)
		return
	}
	if !a.sessions.Elevate(creds.Value, token) {
		authn.ClearSessionCookie(w, r, a.runtime != nil && a.runtime.TrustProxyHeaders, cookieSecureSetting(a.runtime))
		w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
		httpx.ErrorCode(w, http.StatusUnauthorized, "bad_token", "unauthorized", false, nil)
		return
	}

	authn.AuditLog(r, "auth.session_elevated", "elevationWindowSec", int(a.sessions.ElevationWindow().Seconds()))
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"elevationWindowSec": int(a.sessions.ElevationWindow().Seconds()),
	})
}

func cookieSecureSetting(cfg *config.RuntimeConfig) *bool {
	if cfg == nil {
		return nil
	}
	return cfg.CookieSecure
}

func (a *AuthAPI) requiresHTTPSForDashboardSession(r *http.Request) bool {
	if a == nil || a.runtime == nil || a.runtime.CookieSecure == nil || !*a.runtime.CookieSecure {
		return false
	}
	return !authn.RequestIsHTTPS(r, a.runtime.TrustProxyHeaders)
}

func secondsCeil(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	sec := int(d / time.Second)
	if d%time.Second != 0 {
		sec++
	}
	if sec <= 0 {
		return 1
	}
	return sec
}
