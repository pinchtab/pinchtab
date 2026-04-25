package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	streamMu          sync.Mutex
	streamConnections = map[string]int{}
)

const (
	maxConcurrentStreamRequestsPerHost = 8
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
			// Inject agent identity into request headers for activity tracking
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

func sessionRequestAllowed(r *http.Request, sess *session.Session) bool {
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	path := strings.TrimSpace(r.URL.Path)
	if method == http.MethodPost && sessionRevokePath(path) {
		return true
	}
	if sessionAdminRoute(method, path) {
		return false
	}
	if sess == nil {
		return false
	}

	grants := normalizedSessionGrants(sess.Grants)
	if len(grants) == 0 || grants["*"] {
		return true
	}

	for grant := range grants {
		if sessionGrantAllows(grant, method, path) {
			return true
		}
	}
	return false
}

func normalizedSessionGrants(grants []string) map[string]bool {
	out := make(map[string]bool, len(grants))
	for _, grant := range grants {
		normalized := strings.ToLower(strings.TrimSpace(grant))
		if normalized == "" {
			continue
		}
		out[normalized] = true
	}
	return out
}

func sessionAdminRoute(method, path string) bool {
	switch {
	case method == http.MethodGet && path == "/api/config":
		return true
	case method == http.MethodPut && path == "/api/config":
		return true
	case method == http.MethodPost && path == "/shutdown":
		return true
	case method == http.MethodPost && (path == "/browser/restart" || path == "/ensure-chrome"):
		return true
	case method == http.MethodPost && path == "/fingerprint/rotate":
		return true
	case method == http.MethodGet && path == "/api/events":
		return true
	case method == http.MethodGet && path == "/api/metrics":
		return true
	case method == http.MethodGet && path == "/api/agents":
		return true
	case method == http.MethodGet && strings.HasPrefix(path, "/api/agents/") && !strings.HasSuffix(path, "/events"):
		return true
	case path == "/sessions" || strings.HasPrefix(path, "/sessions/"):
		return path != "/sessions/me"
	case path == "/instances" || strings.HasPrefix(path, "/instances/"):
		return true
	case path == "/profiles" || strings.HasPrefix(path, "/profiles/"):
		return true
	case method == http.MethodGet && path == "/cache/status":
		return true
	case method == http.MethodPost && path == "/cache/clear":
		return true
	default:
		return false
	}
}

func sessionRevokePath(path string) bool {
	if !strings.HasPrefix(path, "/sessions/") || !strings.HasSuffix(path, "/revoke") {
		return false
	}
	sessionID := strings.TrimSuffix(strings.TrimPrefix(path, "/sessions/"), "/revoke")
	return strings.Trim(sessionID, "/") != ""
}

func sessionGrantAllows(grant, method, path string) bool {
	switch grant {
	case "browse":
		return sessionBrowseGrantAllows(method, path)
	case "network":
		return sessionNetworkGrantAllows(method, path)
	case "media":
		return sessionMediaGrantAllows(method, path)
	case "cookies":
		return sessionCookiesGrantAllows(method, path)
	case "clipboard":
		return sessionClipboardGrantAllows(method, path)
	case "evaluate":
		return sessionEvaluateGrantAllows(method, path)
	case "storage":
		return sessionStorageGrantAllows(method, path)
	case "console":
		return sessionConsoleGrantAllows(method, path)
	case "solve":
		return sessionSolveGrantAllows(method, path)
	case "tasks":
		return sessionTasksGrantAllows(method, path)
	case "activity":
		return sessionActivityGrantAllows(method, path)
	default:
		return false
	}
}

func sessionBrowseGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/tabs",
			path == "/navigate",
			path == "/action",
			path == "/snapshot",
			path == "/screenshot",
			path == "/text",
			path == "/openapi.json",
			path == "/help",
			path == "/health",
			path == "/sessions/me":
			return true
		case tabRouteHasSuffix(path, "/snapshot"),
			tabRouteHasSuffix(path, "/screenshot"),
			tabRouteHasSuffix(path, "/text"),
			tabRouteHasSuffix(path, "/metrics"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/tab",
			path == "/navigate",
			path == "/back",
			path == "/forward",
			path == "/reload",
			path == "/action",
			path == "/actions",
			path == "/macro",
			path == "/find",
			path == "/wait",
			path == "/dialog",
			path == "/lock",
			path == "/unlock":
			return true
		case tabRouteHasSuffix(path, "/navigate"),
			tabRouteHasSuffix(path, "/back"),
			tabRouteHasSuffix(path, "/forward"),
			tabRouteHasSuffix(path, "/reload"),
			tabRouteHasSuffix(path, "/action"),
			tabRouteHasSuffix(path, "/actions"),
			tabRouteHasSuffix(path, "/find"),
			tabRouteHasSuffix(path, "/wait"),
			tabRouteHasSuffix(path, "/dialog"),
			tabRouteHasSuffix(path, "/lock"),
			tabRouteHasSuffix(path, "/unlock"):
			return true
		}
	}
	return false
}

func sessionNetworkGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/network",
			path == "/network/stream",
			path == "/network/export",
			path == "/network/export/stream":
			return true
		case strings.HasPrefix(path, "/network/"):
			return true
		case tabRouteHasSuffix(path, "/network"),
			tabRouteHasSuffix(path, "/network/stream"),
			tabRouteHasSuffix(path, "/network/export"),
			tabRouteHasSuffix(path, "/network/export/stream"):
			return true
		case strings.HasPrefix(path, "/tabs/") && strings.Contains(path, "/network/"):
			return true
		}
	case http.MethodPost:
		return path == "/network/clear"
	}
	return false
}

func sessionMediaGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/pdf",
			path == "/download",
			path == "/screencast",
			path == "/screencast/tabs":
			return true
		case tabRouteHasSuffix(path, "/pdf"),
			tabRouteHasSuffix(path, "/download"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/pdf",
			path == "/upload":
			return true
		case tabRouteHasSuffix(path, "/pdf"),
			tabRouteHasSuffix(path, "/upload"):
			return true
		}
	}
	return false
}

func sessionCookiesGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet, http.MethodPost:
		return path == "/cookies" || tabRouteHasSuffix(path, "/cookies")
	default:
		return false
	}
}

func sessionClipboardGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/clipboard/read" || path == "/clipboard/paste"
	case http.MethodPost:
		return path == "/clipboard/write" || path == "/clipboard/copy"
	default:
		return false
	}
}

func sessionEvaluateGrantAllows(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	return path == "/evaluate" || tabRouteHasSuffix(path, "/evaluate")
}

func sessionStorageGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/storage" || path == "/state/list" || path == "/state/show"
	case http.MethodPost:
		return path == "/storage" || path == "/state/save" || path == "/state/load" || path == "/state/clean"
	case http.MethodDelete:
		return path == "/storage" || path == "/state"
	default:
		return false
	}
}

func sessionConsoleGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/console" || path == "/errors"
	case http.MethodPost:
		return path == "/console/clear" || path == "/errors/clear"
	default:
		return false
	}
}

func sessionSolveGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/solvers" || path == "/config/autosolver"
	case http.MethodPost:
		switch {
		case path == "/solve" || strings.HasPrefix(path, "/solve/"):
			return true
		case tabRouteHasSuffix(path, "/solve") || (strings.HasPrefix(path, "/tabs/") && strings.Contains(path, "/solve/")):
			return true
		}
	}
	return false
}

func sessionTasksGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/tasks" || path == "/scheduler/stats" || strings.HasPrefix(path, "/tasks/")
	case http.MethodPost:
		return path == "/tasks" || path == "/tasks/batch" || (strings.HasPrefix(path, "/tasks/") && strings.HasSuffix(path, "/cancel"))
	default:
		return false
	}
}

func sessionActivityGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/api/activity" || (strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"))
	case http.MethodPost:
		return strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events")
	default:
		return false
	}
}

func tabRouteHasSuffix(path, suffix string) bool {
	return strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, suffix)
}

func isPublicDashboardPath(path string) bool {
	switch path {
	case "/", "/login", "/dashboard", "/dashboard/":
		return true
	}
	return strings.HasPrefix(path, "/dashboard/") || path == "/dashboard/favicon.png"
}

func isPublicAuthPath(path string) bool {
	switch path {
	case "/api/auth/login", "/api/auth/logout":
		return true
	default:
		return false
	}
}

func cookieAuthAllowed(r *http.Request) bool {
	path := strings.TrimSpace(r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		switch {
		case path == "/health",
			path == "/metrics",
			path == "/api/metrics",
			path == "/api/activity",
			path == "/api/agents",
			path == "/api/events",
			path == "/api/config",
			path == "/sessions",
			strings.HasPrefix(path, "/sessions/"),
			path == "/profiles",
			path == "/instances",
			path == "/instances/tabs",
			path == "/instances/metrics":
			return true
		case strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/tabs"),
			strings.HasPrefix(path, "/api/agents/") && !strings.HasSuffix(path, "/events"),
			strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"),
			strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/logs"),
			strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/logs/stream"),
			strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/proxy/screencast"),
			strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, "/screenshot"),
			strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, "/pdf"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/api/auth/elevate":
			return true
		case strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"):
			return true
		case path == "/action":
			return true
		case path == "/instances/start":
			return true
		case path == "/instances/launch":
			return true
		case strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, "/close"):
			return true
		case strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/stop"):
			return true
		case path == "/profiles":
			return true
		}
	case http.MethodPut:
		return path == "/api/config"
	case http.MethodPatch:
		return strings.HasPrefix(path, "/profiles/")
	case http.MethodDelete:
		return strings.HasPrefix(path, "/profiles/")
	}
	return false
}

func cookieElevationRequired(r *http.Request, cfg *config.RuntimeConfig) bool {
	if cfg == nil || !cfg.Sessions.Dashboard.RequireElevation {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)
	switch r.Method {
	case http.MethodPut:
		return path == "/api/config"
	case http.MethodPost:
		return path == "/shutdown"
	}
	return false
}

func CorsMiddleware(cfg *config.RuntimeConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedOrigin := corsAllowedOrigin(cfg, r)
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			if allowedOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			if strings.TrimSpace(r.Header.Get("Origin")) != "" && allowedOrigin == "" && strings.TrimSpace(cfg.Token) != "" {
				httpx.ErrorCode(w, 403, "cors_forbidden", "cross-origin requests are disabled when auth is enabled", false, nil)
				return
			}
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func corsAllowedOrigin(cfg *config.RuntimeConfig, r *http.Request) string {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return ""
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return "*"
	}
	if sameOriginRequest(origin, r, cfg.TrustProxyHeaders) {
		return origin
	}
	return ""
}

func sameOriginRequest(origin string, r *http.Request, trustProxy ...bool) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	trust := len(trustProxy) > 0 && trustProxy[0]
	return strings.EqualFold(parsed.Scheme, requestScheme(r, trust)) && strings.EqualFold(parsed.Host, requestHost(r, trust))
}

func cookieOriginAllowed(r *http.Request, trustProxy bool) bool {
	if isWebSocketUpgrade(r) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		return origin != "" && sameOriginRequest(origin, r, trustProxy)
	}

	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return sameOriginRequest(origin, r, trustProxy)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return sameOriginRequest(referer, r, trustProxy)
	}
	return false
}

func isWebSocketUpgrade(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func cookieSecureSetting(cfg *config.RuntimeConfig) *bool {
	if cfg == nil {
		return nil
	}
	return cfg.CookieSecure
}

func requestScheme(r *http.Request, trustProxy bool) string {
	if r == nil {
		return "http"
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
			return strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
		}
		if forwarded := strings.TrimSpace(r.Header.Get("Forwarded")); forwarded != "" {
			for _, part := range strings.Split(forwarded, ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
				if !ok || !strings.EqualFold(key, "proto") {
					continue
				}
				return strings.ToLower(strings.Trim(value, `"`))
			}
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func requestHost(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
			return strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
		if forwarded := strings.TrimSpace(r.Header.Get("Forwarded")); forwarded != "" {
			for _, part := range strings.Split(forwarded, ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
				if !ok || !strings.EqualFold(key, "host") {
					continue
				}
				return strings.Trim(value, `"`)
			}
		}
	}
	return strings.TrimSpace(r.Host)
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

var (
	rateMu             sync.Mutex
	rateBuckets        = map[string][]time.Time{}
	rateLimiterStarted sync.Once
)

const (
	rateLimitWindow  = 10 * time.Second
	rateLimitMaxReq  = 300
	evictionInterval = 30 * time.Second
)

func RateLimitMiddleware(next http.Handler) http.Handler {
	startRateLimiterJanitor(rateLimitWindow, evictionInterval)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLongLivedStreamRequest(r) {
			host := authn.ClientIP(r)
			if !acquireStreamConnection(host) {
				atomic.AddUint64(&metricRateLimited, 1)
				httpx.ErrorCode(w, 429, "stream_limit_reached", "too many concurrent streaming connections", true, map[string]any{
					"maxConcurrent": maxConcurrentStreamRequestsPerHost,
				})
				return
			}
			defer releaseStreamConnection(host)
			next.ServeHTTP(w, r)
			return
		}

		host := authn.ClientIP(r)

		now := time.Now()
		rateMu.Lock()
		hits := rateBuckets[host]
		filtered := hits[:0]
		for _, t := range hits {
			if now.Sub(t) < rateLimitWindow {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) >= rateLimitMaxReq {
			rateBuckets[host] = filtered
			rateMu.Unlock()
			atomic.AddUint64(&metricRateLimited, 1)
			httpx.ErrorCode(w, 429, "rate_limited", "too many requests", true, map[string]any{"windowSec": int(rateLimitWindow.Seconds()), "max": rateLimitMaxReq})
			return
		}
		rateBuckets[host] = append(filtered, now)
		rateMu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func isLongLivedStreamRequest(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)
	switch {
	case path == "/api/events":
		return true
	case strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"):
		return true
	case strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/logs/stream"):
		return true
	default:
		return false
	}
}

func acquireStreamConnection(host string) bool {
	streamMu.Lock()
	defer streamMu.Unlock()

	if streamConnections[host] >= maxConcurrentStreamRequestsPerHost {
		return false
	}
	streamConnections[host]++
	return true
}

func releaseStreamConnection(host string) {
	streamMu.Lock()
	defer streamMu.Unlock()

	current := streamConnections[host]
	if current <= 1 {
		delete(streamConnections, host)
		return
	}
	streamConnections[host] = current - 1
}

func startRateLimiterJanitor(window, interval time.Duration) {
	rateLimiterStarted.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for now := range ticker.C {
				evictStaleRateBuckets(now, window)
			}
		}()
	})
}

func evictStaleRateBuckets(now time.Time, window time.Duration) {
	rateMu.Lock()
	defer rateMu.Unlock()
	for host, hits := range rateBuckets {
		filtered := hits[:0]
		for _, t := range hits {
			if now.Sub(t) < window {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			delete(rateBuckets, host)
		} else {
			rateBuckets[host] = filtered
		}
	}
}
