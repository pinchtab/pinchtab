package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const (
	maxConcurrentStreamRequestsPerHost = 8
	rateLimitWindow                    = 10 * time.Second
	defaultRateLimitMaxReq             = 3000
)

var rateLimitMaxReq = func() int {
	if v, err := strconv.Atoi(os.Getenv("PINCHTAB_RATE_LIMIT_MAX")); err == nil && v > 0 {
		return v
	}
	return defaultRateLimitMaxReq
}()

var (
	streamMu          sync.Mutex
	streamConnections = map[string]int{}

	requestLimiter = newRequestLimiter()
)

func newRequestLimiter() *authn.AttemptLimiter {
	return authn.NewAttemptLimiter(authn.AttemptLimiterConfig{Window: rateLimitWindow, MaxAttempts: rateLimitMaxReq})
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := authn.ClientIP(r)
		if isLongLivedStreamRequest(r) {
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

		if allowed, retryAfter := requestLimiter.Take(host); !allowed {
			atomic.AddUint64(&metricRateLimited, 1)
			w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
			httpx.ErrorCode(w, 429, "rate_limited", "too many requests", true, map[string]any{"windowSec": int(rateLimitWindow.Seconds()), "max": rateLimitMaxReq})
			return
		}
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
