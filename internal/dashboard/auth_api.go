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
	"github.com/pinchtab/pinchtab/internal/remedy"
)

type AuthAPI struct {
	live         *config.Live
	sessions     *browsersession.Manager
	loginLimiter *authn.AttemptLimiter
}

func NewAuthAPI(live *config.Live, sessions *browsersession.Manager) *AuthAPI {
	return &AuthAPI{
		live:     live,
		sessions: sessions,
		loginLimiter: authn.NewAttemptLimiter(authn.AttemptLimiterConfig{
			Window:      authn.DefaultLoginRateLimitWindow,
			MaxAttempts: authn.DefaultLoginRateLimitMaxAttempt,
		}),
	}
}

func (a *AuthAPI) cfg() *config.RuntimeConfig {
	if a == nil {
		return nil
	}
	return a.live.Get()
}

func (a *AuthAPI) token() string {
	cfg := a.cfg()
	if cfg == nil {
		return ""
	}
	return cfg.Token
}

func cookieTrustsProxy(cfg *config.RuntimeConfig) bool {
	return cfg != nil && cfg.TrustProxyHeaders
}

func (a *AuthAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", a.HandleLogin)
	mux.HandleFunc("POST /api/auth/elevate", a.HandleElevate)
	mux.HandleFunc("POST /api/auth/logout", a.HandleLogout)
}

func (a *AuthAPI) HandleLogin(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(a.token())
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
		httpx.Unauthorized(w, httpx.CodeMissingToken, "")
		return
	}

	if a.requiresHTTPSForDashboardSession(r) {
		// No remedy: the fix is a choice between three postures, not one command, and an
		// absent field says that truthfully. The guidance survives in the hint.
		httpx.ErrorCode(w, http.StatusBadRequest, "secure_cookie_requires_https", "server.cookieSecure=true requires HTTPS for dashboard login", false,
			remedy.Details("Use HTTPS directly or through a trusted reverse proxy, or set server.cookieSecure back to auto/false for plain HTTP local use. If TLS terminates in front of PinchTab, also enable server.trustProxyHeaders so forwarded https requests are recognized.",
				remedy.None))
		return
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		if a.loginLimiter != nil {
			a.loginLimiter.RecordFailure(clientIP)
		}
		authn.ClearSessionCookie(w, r, cookieTrustsProxy(a.cfg()), cookieSecureSetting(a.cfg()))
		authn.AuditWarn(r, "auth.login_failed", "reason", "bad_token")
		httpx.Unauthorized(w, httpx.CodeBadToken, provided)
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
	authn.SetSessionCookie(w, r, sessionID, a.sessions.MaxLifetime(), cookieTrustsProxy(a.cfg()), cookieSecureSetting(a.cfg()))
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
	authn.ClearSessionCookie(w, r, cookieTrustsProxy(a.cfg()), cookieSecureSetting(a.cfg()))
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *AuthAPI) HandleElevate(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(a.token())
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
		httpx.Unauthorized(w, httpx.CodeMissingToken, "")
		return
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		authn.AuditWarn(r, "auth.elevation_failed", "reason", "bad_token")
		httpx.Unauthorized(w, httpx.CodeBadToken, provided)
		return
	}
	if !a.sessions.Elevate(creds.Value, token) {
		authn.ClearSessionCookie(w, r, cookieTrustsProxy(a.cfg()), cookieSecureSetting(a.cfg()))
		httpx.Unauthorized(w, httpx.CodeBadToken, "")
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
	cfg := a.cfg()
	if cfg == nil || cfg.CookieSecure == nil || !*cfg.CookieSecure {
		return false
	}
	return !authn.RequestIsHTTPS(r, cfg.TrustProxyHeaders)
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
