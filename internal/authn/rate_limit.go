package authn

import (
	"strings"
	"sync"
	"time"
)

const (
	DefaultLoginRateLimitWindow     = 10 * time.Minute
	DefaultLoginRateLimitMaxAttempt = 10
)

type AttemptLimiterConfig struct {
	Window      time.Duration
	MaxAttempts int
}

type AttemptLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	maxAttempts int
	attempts    map[string][]time.Time
	now         func() time.Time
}

func NewAttemptLimiter(cfg AttemptLimiterConfig) *AttemptLimiter {
	window := cfg.Window
	if window <= 0 {
		window = DefaultLoginRateLimitWindow
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultLoginRateLimitMaxAttempt
	}
	return &AttemptLimiter{
		window:      window,
		maxAttempts: maxAttempts,
		attempts:    make(map[string][]time.Time),
		now:         time.Now,
	}
}

func (l *AttemptLimiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return true, 0
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	filtered := l.pruneLocked(key, now)
	if len(filtered) < l.maxAttempts {
		return true, 0
	}

	retryAfter := l.window - now.Sub(filtered[0])
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter
}

func (l *AttemptLimiter) RecordFailure(key string) {
	if l == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	filtered := l.pruneLocked(key, now)
	l.attempts[key] = append(filtered, now)
}

func (l *AttemptLimiter) Reset(key string) {
	if l == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *AttemptLimiter) Window() time.Duration {
	if l == nil {
		return DefaultLoginRateLimitWindow
	}
	return l.window
}

func (l *AttemptLimiter) MaxAttempts() int {
	if l == nil {
		return DefaultLoginRateLimitMaxAttempt
	}
	return l.maxAttempts
}

func (l *AttemptLimiter) pruneLocked(key string, now time.Time) []time.Time {
	hits := l.attempts[key]
	if len(hits) == 0 {
		return nil
	}
	filtered := hits[:0]
	for _, ts := range hits {
		if now.Sub(ts) < l.window {
			filtered = append(filtered, ts)
		}
	}
	if len(filtered) == 0 {
		delete(l.attempts, key)
		return nil
	}
	l.attempts[key] = filtered
	return filtered
}
