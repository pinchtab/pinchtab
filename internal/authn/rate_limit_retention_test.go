package authn

import (
	"fmt"
	"testing"
	"time"
)

// Attempts are pruned only when the same key is seen again, so a burst of
// failed logins from many distinct peers would otherwise leave one map entry
// per peer alive forever in a long-running daemon.
func TestAttemptLimiterDoesNotRetainExpiredKeys(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewAttemptLimiter(AttemptLimiterConfig{Window: time.Minute, MaxAttempts: 3})
	l.now = func() time.Time { return now }

	for i := range 1000 {
		l.RecordFailure(fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}

	now = now.Add(time.Hour)
	l.RecordFailure("192.0.2.1")

	l.mu.Lock()
	retained := len(l.attempts)
	l.mu.Unlock()

	if retained > 1 {
		t.Errorf("limiter retains %d keys after all but one expired, want 1", retained)
	}
}

// Sweeping must not drop keys that are still inside the window, which would
// hand an attacker a free reset of their attempt count.
func TestAttemptLimiterKeepsLiveKeysWhileSweeping(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewAttemptLimiter(AttemptLimiterConfig{Window: time.Minute, MaxAttempts: 3})
	l.now = func() time.Time { return now }

	for range 3 {
		l.RecordFailure("10.0.0.1")
	}
	if allowed, _ := l.Allow("10.0.0.1"); allowed {
		t.Fatal("key should be blocked after hitting maxAttempts")
	}

	// Enough later to trigger a sweep, but still inside the window for this key.
	now = now.Add(50 * time.Second)
	l.RecordFailure("192.0.2.1")

	if allowed, _ := l.Allow("10.0.0.1"); allowed {
		t.Error("a sweep dropped a key that is still within its window")
	}
}
