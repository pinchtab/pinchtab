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

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

var (
	metricRequestsTotal   uint64
	metricRequestsFailed  uint64
	metricRequestLatencyN uint64
	metricRateLimited     uint64
	metricStaleRefRetries uint64
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

func AuthMiddleware(cfg *config.RuntimeConfig, next http.Handler) http.Handler {
	return AuthMiddlewareWithSessions(cfg, nil, next)
}

func AuthMiddlewareWithSessions(cfg *config.RuntimeConfig, sessions *authn.SessionManager, next http.Handler) http.Handler {
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
			authn.ClearSessionCookie(w, r)
			w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="missing_token"`)
			httpx.ErrorCode(w, 401, "missing_token", "unauthorized", false, nil)
			return
		}

		switch creds.Method {
		case authn.MethodHeader:
			if subtle.ConstantTimeCompare([]byte(creds.Value), []byte(token)) != 1 {
				authn.ClearSessionCookie(w, r)
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
				httpx.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
				return
			}
		case authn.MethodCookie:
			if !cookieOriginAllowed(r) {
				httpx.ErrorCode(w, http.StatusForbidden, "origin_forbidden", "same-origin browser request required for session authentication", false, map[string]any{
					"sameOriginRequired": true,
				})
				return
			}
			if sessions == nil || !sessions.Validate(creds.Value, token) {
				authn.ClearSessionCookie(w, r)
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
				httpx.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
				return
			}
			if !cookieAuthAllowed(r) {
				httpx.ErrorCode(w, 403, "header_auth_required", "authorization header required for this endpoint", false, nil)
				return
			}
			if cookieElevationRequired(r) && !sessions.IsElevated(creds.Value, token) {
				authn.AuditWarn(r, "auth.elevation_required", "elevationWindowSec", int(sessions.ElevationWindow().Seconds()))
				httpx.ErrorCode(w, 403, "elevation_required", "re-enter API token to continue", false, map[string]any{
					"elevationWindowSec": int(sessions.ElevationWindow().Seconds()),
				})
				return
			}
		default:
			authn.ClearSessionCookie(w, r)
			w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
			httpx.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
			return
		}
		next.ServeHTTP(w, r)
	})
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
			path == "/api/activity",
			path == "/api/events",
			path == "/api/config",
			path == "/profiles",
			path == "/instances",
			path == "/instances/tabs",
			path == "/instances/metrics":
			return true
		case strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/tabs"),
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
		case path == "/instances/launch":
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

func cookieElevationRequired(r *http.Request) bool {
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
	if sameOriginRequest(origin, r) {
		return origin
	}
	return ""
}

func sameOriginRequest(origin string, r *http.Request) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, requestScheme(r)) && strings.EqualFold(parsed.Host, requestHost(r))
}

func cookieOriginAllowed(r *http.Request) bool {
	if isWebSocketUpgrade(r) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		return origin != "" && sameOriginRequest(origin, r)
	}

	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return sameOriginRequest(origin, r)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return sameOriginRequest(referer, r)
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

func requestScheme(r *http.Request) string {
	if r != nil && r.TLS != nil {
		return "https"
	}
	return "http"
}

func requestHost(r *http.Request) string {
	if r == nil {
		return ""
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
	rateLimitMaxReq  = 120
	evictionInterval = 30 * time.Second
)

func RateLimitMiddleware(next http.Handler) http.Handler {
	startRateLimiterJanitor(rateLimitWindow, evictionInterval)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimSpace(r.URL.Path)
		if p == "/health" || p == "/metrics" || strings.HasPrefix(p, "/health/") || strings.HasPrefix(p, "/metrics/") {
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
