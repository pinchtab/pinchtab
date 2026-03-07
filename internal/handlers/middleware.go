package handlers

import (
	"crypto/rand"
	"crypto/subtle"
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
	metricPanicsRecovered uint64
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Token != "" {
			auth := r.Header.Get("Authorization")
			qToken := r.URL.Query().Get("token")

			if auth == "" && qToken == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="pinchtab", error="missing_token"`)
				web.ErrorCode(w, 401, "missing_token", "unauthorized", false, nil)
				return
			}

			provided := strings.TrimPrefix(auth, "Bearer ")
			if provided == auth { // "Bearer " prefix was not present
				if qToken != "" {
					provided = qToken
				} else {
					provided = auth
				}
			}

			if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.Token)) != 1 {
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
			if now.Sub(t) < rateLimitWindow {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) >= rateLimitMaxReq {
			rateBuckets[host] = filtered
			rateMu.Unlock()
			atomic.AddUint64(&metricRateLimited, 1)
			web.ErrorCode(w, 429, "rate_limited", "too many requests", true, map[string]any{"windowSec": int(rateLimitWindow.Seconds()), "max": rateLimitMaxReq})
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
