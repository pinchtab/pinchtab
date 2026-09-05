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

func TestAttemptLimiterTakeCountsAndRefusesAtomically(t *testing.T) {
	limiter := NewAttemptLimiter(AttemptLimiterConfig{Window: time.Minute, MaxAttempts: 2})
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if allowed, _ := limiter.Take("10.0.0.1"); !allowed {
			t.Fatalf("take %d refused before the cap", i+1)
		}
	}
	allowed, retryAfter := limiter.Take("10.0.0.1")
	if allowed || retryAfter != time.Minute {
		t.Fatalf("third take: allowed=%v retryAfter=%v, want refused with the full window", allowed, retryAfter)
	}
	now = now.Add(time.Minute + time.Second)
	if allowed, _ := limiter.Take("10.0.0.1"); !allowed {
		t.Fatal("a refused take must not extend the window")
	}
}

func TestAttemptLimiterSweepDropsKeysWhoseHitsAllAgedOut(t *testing.T) {
	limiter := NewAttemptLimiter(AttemptLimiterConfig{Window: 10 * time.Second, MaxAttempts: 100})
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	limiter.RecordFailure("stale-only")
	now = now.Add(5 * time.Second)
	limiter.RecordFailure("mixed")
	now = now.Add(6 * time.Second)
	limiter.RecordFailure("fresh")

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if _, ok := limiter.attempts["stale-only"]; ok {
		t.Fatal("a key whose every hit aged out must be swept when another key records")
	}
	if got := len(limiter.attempts["mixed"]); got != 1 {
		t.Fatalf("mixed kept %d hits, want 1", got)
	}
}
