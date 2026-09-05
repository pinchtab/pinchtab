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
	lastSweep   time.Time
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
	return l.allowedLocked(l.pruneLocked(key, now), now)
}

func (l *AttemptLimiter) Take(key string) (bool, time.Duration) {
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
	if allowed, retryAfter := l.allowedLocked(filtered, now); !allowed {
		return false, retryAfter
	}
	l.sweepLocked(now)
	l.attempts[key] = append(filtered, now)
	return true, 0
}

func (l *AttemptLimiter) allowedLocked(hits []time.Time, now time.Time) (bool, time.Duration) {
	if len(hits) < l.maxAttempts {
		return true, 0
	}
	retryAfter := l.window - now.Sub(hits[0])
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

	l.sweepLocked(now)
	filtered := l.pruneLocked(key, now)
	l.attempts[key] = append(filtered, now)
}

// sweepLocked drops every key whose attempts have all aged out. pruneLocked
// only ever touches the key being looked at, so without this a failed login
// from each of many distinct peers leaves an entry alive indefinitely — the
// key is never revisited, so it is never pruned. Rate-limited by window so a
// burst of failures cannot turn each one into a full map scan.
func (l *AttemptLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for key, hits := range l.attempts {
		live := false
		for _, ts := range hits {
			if now.Sub(ts) < l.window {
				live = true
				break
			}
		}
		if !live {
			delete(l.attempts, key)
		}
	}
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

func (l *AttemptLimiter) TrackedKeys() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.attempts)
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
