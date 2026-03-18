package authn

import (
	"testing"
	"time"
)

func TestAttemptLimiterBlocksAfterMaxFailures(t *testing.T) {
	limiter := NewAttemptLimiter(AttemptLimiterConfig{
		Window:      time.Minute,
		MaxAttempts: 2,
	})
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	if allowed, _ := limiter.Allow("127.0.0.1"); !allowed {
		t.Fatal("expected first check to be allowed")
	}

	limiter.RecordFailure("127.0.0.1")
	limiter.RecordFailure("127.0.0.1")

	allowed, retryAfter := limiter.Allow("127.0.0.1")
	if allowed {
		t.Fatal("expected limiter to block after max failures")
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}
}

func TestAttemptLimiterResetsAndExpires(t *testing.T) {
	limiter := NewAttemptLimiter(AttemptLimiterConfig{
		Window:      time.Minute,
		MaxAttempts: 1,
	})
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	limiter.RecordFailure("127.0.0.1")
	if allowed, _ := limiter.Allow("127.0.0.1"); allowed {
		t.Fatal("expected limiter to block after recorded failure")
	}

	limiter.Reset("127.0.0.1")
	if allowed, _ := limiter.Allow("127.0.0.1"); !allowed {
		t.Fatal("expected reset to clear failures")
	}

	limiter.RecordFailure("127.0.0.1")
	now = now.Add(2 * time.Minute)
	if allowed, _ := limiter.Allow("127.0.0.1"); !allowed {
		t.Fatal("expected failures to expire after the window")
	}
}
