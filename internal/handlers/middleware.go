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
	"github.com/pinchtab/pinchtab/internal/remedy"
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
	backgroundHealthPath    = "/health/background"
	backgroundHealthHeader  = "PinchTab-Background-Marker"
)

// requestLogLevel maps the answered status onto a severity an operator can route on. Every
// request used to log at Info, so a server returning 500s looked exactly like a healthy one
// to any level-based alert, dashboard or log shipper — `grep level=ERROR` found nothing.
//
// A recorded reason raises the level on its own. Some endpoints answer 200 while
// reporting a failure inside the envelope — a batch or macro run whose steps all
// failed — and keying only on the status made those runs log as healthy traffic.
func requestLogLevel(status int, reasonRecorded bool) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400, reasonRecorded:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// requestFailed is the one definition of "this request failed" for the recording
// channels: the status, or a reason the handler published beside a success status.
// The producer that wrote the response is the only thing that knows the second
// case, which is why it is a recorded fact rather than something re-derived here.
func requestFailed(sw *httpx.StatusWriter) bool {
	return sw.Code >= 400 || sw.FailureMessage != ""
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &httpx.StatusWriter{ResponseWriter: w, Code: 200, StripFailureHeaders: !IsTrustedInternalProxy(r)}
		next.ServeHTTP(sw, r)
		ms := uint64(time.Since(start).Milliseconds())
		atomic.AddUint64(&metricRequestsTotal, 1)
		atomic.AddUint64(&metricRequestLatencyN, ms)
		if requestFailed(sw) {
			atomic.AddUint64(&metricRequestsFailed, 1)
			recordFailureEvent(FailureEvent{
				Time:      time.Now(),
				RequestID: w.Header().Get(httpx.RequestIDHeader),
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    sw.Code,
				Type:      "http_error",
				Code:      sw.FailureCode,
				Message:   sw.FailureMessage,
			})
		}
		attrs := []any{
			"requestId", w.Header().Get(httpx.RequestIDHeader),
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.Code,
			"ms", ms,
		}
		if sw.FailureMessage != "" {
			attrs = append(attrs, "code", sw.FailureCode, "error", sw.FailureMessage)
		}
		slog.Log(r.Context(), requestLogLevel(sw.Code, sw.FailureMessage != ""), "request", attrs...)
	})
}

// SecurityHeadersMiddleware and every other middleware here take the publication
// point rather than a config, and resolve it PER REQUEST. A middleware is built
// once at startup and serves for the life of the process, so one that closed over
// the boot *RuntimeConfig would enforce boot values forever: a save publishes a
// new value and never writes the old object. That froze trustProxyHeaders,
// cookieSecure and sessions.dashboard.requireElevation while the dashboard
// reported the save applied.
func SecurityHeadersMiddleware(live *config.Live, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", defaultCSP)
		trustProxy := cfg != nil && cfg.TrustProxyHeaders
		if authn.RequestIsHTTPS(r, trustProxy) {
			w.Header().Set("Strict-Transport-Security", strictTransportSecurity)
		}
		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(live *config.Live, next http.Handler) http.Handler {
	return AuthMiddlewareWithSessions(live, nil, nil, next)
}

func AuthMiddlewareWithSessions(live *config.Live, sessions *browsersession.Manager, agentSessions *session.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		if isPublicDashboardPath(r.URL.Path) || isPublicAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if backgroundHealthProbeAllowed(cfg, r) {
			next.ServeHTTP(w, r)
			return
		}
		if cfg == nil {
			httpx.ErrorCode(w, http.StatusServiceUnavailable, "token_required", "server token is not configured", false, nil)
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
			httpx.Unauthorized(w, httpx.CodeMissingToken, "")
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
			if refusal, refused := sessionRequestRefusal(r, sess); refused {
				details := remedy.Details(refusal.hint, refusal.remedy)
				details["safeControlledEnvironmentOnly"] = true
				httpx.ErrorCode(w, http.StatusForbidden, "session_scope_forbidden", "agent session is not allowed to access this endpoint", false, details)
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
				httpx.Unauthorized(w, httpx.CodeBadToken, creds.Value)
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
				httpx.Unauthorized(w, httpx.CodeBadToken, "")
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
			httpx.Unauthorized(w, httpx.CodeBadToken, creds.Value)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func backgroundHealthProbeAllowed(cfg *config.RuntimeConfig, r *http.Request) bool {
	if cfg == nil || r.Method != http.MethodGet || r.URL.Path != backgroundHealthPath {
		return false
	}
	marker := strings.TrimSpace(cfg.BackgroundMarker)
	got := strings.TrimSpace(r.Header.Get(backgroundHealthHeader))
	if marker == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(marker)) == 1
}

// StripInternalHeadersMiddleware removes any X-PinchTab-* headers that arrived
// from the public network. These headers are reserved for trusted internal
// propagation (orchestrator → instance) after auth, and accepting them from
// public clients would let a bearer-authenticated caller spoof another
// session's identity.
//
// Mount as the outermost middleware on every public-facing server. Trusted
// internal hops re-add the headers after the strip layer.
func StripInternalHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripPinchtabHeaders(r.Header)
		next.ServeHTTP(w, r)
	})
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(httpx.RequestIDHeader)
		if !usableRequestID(rid) {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			rid = hex.EncodeToString(b)
		}
		w.Header().Set(httpx.RequestIDHeader, rid)
		r.Header.Set(httpx.RequestIDHeader, rid)
		next.ServeHTTP(w, r)
	})
}

// maxRequestIDLen bounds an inbound correlation id. A UUID is 36 characters and
// a W3C traceparent 55, so this leaves room for every id a real client sends
// while keeping the header out of the "arbitrary payload" class — the server
// accepts 256 KiB of headers, and this value is copied into every audit and
// access log line for the request, into the dashboard activity stream, and onto
// the proxied hop.
const maxRequestIDLen = 128

// usableRequestID reports whether an inbound X-Request-Id may be adopted as this
// request's correlation id. Honouring a client's id is deliberate — it is what
// makes one request findable in the outer log and the instance log alike — but
// the id is adopted only when it is shaped like one. Anything else is replaced
// by a generated id rather than refused, because the correlation id is not
// something a request should fail on.
func usableRequestID(rid string) bool {
	if rid == "" || len(rid) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(rid); i++ {
		c := rid[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.', c == ':':
		default:
			return false
		}
	}
	return true
}
