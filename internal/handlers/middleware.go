package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/web"
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
		sw := &web.StatusWriter{ResponseWriter: w, Code: 200}
		next.ServeHTTP(sw, r)
		ms := uint64(time.Since(start).Milliseconds())
		atomic.AddUint64(&metricRequestsTotal, 1)
		atomic.AddUint64(&metricRequestLatencyN, ms)
		if sw.Code >= 400 {
			atomic.AddUint64(&metricRequestsFailed, 1)
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Token != "" {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="missing_token"`)
				web.ErrorCode(w, 401, "missing_token", "unauthorized", false, nil)
				return
			}
			if auth != "Bearer "+cfg.Token {
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="bad_token"`)
				web.ErrorCode(w, 401, "bad_token", "unauthorized", false, nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
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
		next.ServeHTTP(w, r)
	})
}

var (
	rateMu      sync.Mutex
	rateBuckets = map[string][]time.Time{}
)

func RateLimitMiddleware(next http.Handler) http.Handler {
	const window = 10 * time.Second
	const maxReq = 120
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimSpace(r.URL.Path)
		if p == "/health" || p == "/metrics" || strings.HasPrefix(p, "/health/") || strings.HasPrefix(p, "/metrics/") {
			next.ServeHTTP(w, r)
			return
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "" {
			host = r.RemoteAddr
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			host = strings.TrimSpace(strings.Split(xff, ",")[0])
		}

		now := time.Now()
		rateMu.Lock()
		hits := rateBuckets[host]
		filtered := hits[:0]
		for _, t := range hits {
			if now.Sub(t) < window {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) >= maxReq {
			rateBuckets[host] = filtered
			rateMu.Unlock()
			atomic.AddUint64(&metricRateLimited, 1)
			web.ErrorCode(w, 429, "rate_limited", "too many requests", true, map[string]any{"windowSec": int(window.Seconds()), "max": maxReq})
			return
		}
		rateBuckets[host] = append(filtered, now)
		rateMu.Unlock()

		next.ServeHTTP(w, r)
	})
}
