package handlers

import (
	"net/http"
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
	rateLimitMaxReq                    = 300
	evictionInterval                   = 30 * time.Second
)

var (
	streamMu          sync.Mutex
	streamConnections = map[string]int{}

	rateMu             sync.Mutex
	rateBuckets        = map[string][]time.Time{}
	rateLimiterStarted sync.Once
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
