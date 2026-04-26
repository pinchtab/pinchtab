package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
)

var (
	metricRequestsTotal   uint64
	metricRequestsFailed  uint64
	metricRequestLatencyN uint64
	metricRateLimited     uint64
	metricStaleRefRetries uint64
)

const (
	defaultCSP              = "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; img-src 'self' data: blob:; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:"
	strictTransportSecurity = "max-age=31536000"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &httpx.StatusWriter{ResponseWriter: w, Code: 200}
		next.ServeHTTP(sw, r)
		ms := uint64(time.Since(start).Milliseconds())
		atomic.AddUint64(&metricRequestsTotal, 1)
		atomic.AddUint64(&metricRequestLatencyN, ms)
		if sw.Code >= 400 {
			atomic.AddUint64(&metricRequestsFailed, 1)
			recordFailureEvent(FailureEvent{
				Time:      time.Now(),
				RequestID: w.Header().Get("X-Request-Id"),
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    sw.Code,
				Type:      "http_error",
			})
		}
		slog.Info("request",
			"requestId", w.Header().Get("X-Request-Id"),
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.Code,
			"ms", ms,
		)
	})
}

func SecurityHeadersMiddleware(cfg *config.RuntimeConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", defaultCSP)
		trustProxy := cfg != nil && cfg.TrustProxyHeaders
		if requestScheme(r, trustProxy) == "https" {
			w.Header().Set("Strict-Transport-Security", strictTransportSecurity)
		}
		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(cfg *config.RuntimeConfig, next http.Handler) http.Handler {
	return AuthMiddlewareWithSessions(cfg, nil, nil, next)
}

func AuthMiddlewareWithSessions(cfg *config.RuntimeConfig, sessions *browsersession.Manager, agentSessions *session.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicDashboardPath(r.URL.Path) || isPublicAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(cfg.Token)
		if token == "" {
			httpx.ErrorCode(w, http.StatusServiceUnavailable, "token_required", "server token is not configured", false, nil)
			return
		}

		creds := authn.CredentialsFromRequest(r)
		if creds.Value == "" {
			authn.ClearSessionCookie(w, r, cfg != nil && cfg.TrustProxyHeaders, cookieSecureSetting(cfg))
			w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="missing_token"`)
			httpx.ErrorCode(w, 401, "missing_token", "unauthorized", false, nil)
			return
		}

		switch creds.Method {
		case authn.MethodSession:
			if agentSessions == nil || !agentSessions.Enabled() {
				httpx.ErrorCode(w, 401, "session_auth_unavailable", "agent session authentication is not enabled", false, nil)
				return
			}
			sess, ok := agentSessions.AuthenticateWithoutTouch(creds.Value)
			if !ok || sess == nil {
				w.Header().Set("WWW-Authenticate", `Session realm="pinchtab", error="bad_session"`)
				httpx.ErrorCode(w, 401, "bad_session", "invalid or expired agent session", false, nil)
				return
			}
			if !sessionRequestAllowed(r, sess) {
				httpx.ErrorCode(w, http.StatusForbidden, "session_scope_forbidden", "agent session is not allowed to access this endpoint", false, map[string]any{
					"safeControlledEnvironmentOnly": true,
				})
				return
			}
			if !agentSessions.Touch(sess.ID) {
				w.Header().Set("WWW-Authenticate", `Session realm="pinchtab", error="bad_session"`)
				httpx.ErrorCode(w, 401, "bad_session", "invalid or expired agent session", false, nil)
				return
			}
			r.Header.Set(activity.HeaderAgentID, sess.AgentID)
			r.Header.Set(activity.HeaderPTSessionID, sess.ID)
			activity.EnrichRequest(r, activity.Update{
				AgentID:   sess.AgentID,
				SessionID: sess.ID,
			})
			r = session.WithSession(r, sess)
		case authn.MethodHeader:
			if subtle.ConstantTimeCompare([]byte(creds.Value), []byte(token)) != 1 {
				authn.ClearSessionCookie(w, r, cfg != nil && cfg.TrustProxyHeaders, cookieSecureSetting(cfg))
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
				httpx.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
				return
			}
		case authn.MethodCookie:
			if !cookieOriginAllowed(r, cfg.TrustProxyHeaders) {
				httpx.ErrorCode(w, http.StatusForbidden, "origin_forbidden", "same-origin browser request required for session authentication", false, map[string]any{
					"sameOriginRequired": true,
				})
				return
			}
			if sessions == nil || !sessions.Validate(creds.Value, token) {
				authn.ClearSessionCookie(w, r, cfg != nil && cfg.TrustProxyHeaders, cookieSecureSetting(cfg))
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
				httpx.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
				return
			}
			if !cookieAuthAllowed(r) {
				httpx.ErrorCode(w, 403, "header_auth_required", "authorization header required for this endpoint", false, nil)
				return
			}
			if cookieElevationRequired(r, cfg) && !sessions.IsElevated(creds.Value, token) {
				authn.AuditWarn(r, "auth.elevation_required", "elevationWindowSec", int(sessions.ElevationWindow().Seconds()))
				httpx.ErrorCode(w, 403, "elevation_required", "re-enter API token to continue", false, map[string]any{
					"elevationWindowSec": int(sessions.ElevationWindow().Seconds()),
				})
				return
			}
		default:
			authn.ClearSessionCookie(w, r, cfg != nil && cfg.TrustProxyHeaders, cookieSecureSetting(cfg))
			w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
			httpx.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			rid = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-Id", rid)
		r.Header.Set("X-Request-Id", rid)
		next.ServeHTTP(w, r)
	})
}
