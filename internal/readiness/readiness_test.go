package readiness

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitUntil_ReadyOnFirstProbe(t *testing.T) {
	calls := 0
	got, err := WaitUntil(context.Background(), time.Second, time.Millisecond, func() (string, bool, error) {
		calls++
		return "ready", true, nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "ready" {
		t.Fatalf("got = %q, want %q", got, "ready")
	}
	if calls != 1 {
		t.Fatalf("probe called %d times, want 1", calls)
	}
}

func TestWaitUntil_ProbeErrorShortCircuits(t *testing.T) {
	sentinel := errors.New("boom")
	calls := 0
	_, err := WaitUntil(context.Background(), time.Second, time.Millisecond, func() (string, bool, error) {
		calls++
		return "", false, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if calls != 1 {
		t.Fatalf("probe called %d times after error, want 1", calls)
	}
}

func TestWaitUntil_NeverReadyReturnsErrNotReady(t *testing.T) {
	calls := 0
	_, err := WaitUntil(context.Background(), 12*time.Millisecond, 5*time.Millisecond, func() (string, bool, error) {
		calls++
		return "", false, nil
	})
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady", err)
	}
	// probe-first with deadline-at-top: probes at ~0, ~5, ~10 (before the 12ms
	// deadline), then exits — at least 2 probes, no extra probe past the deadline.
	if calls < 2 {
		t.Fatalf("probe called %d times, want >= 2", calls)
	}
}

func TestWaitUntil_ContextCancelReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WaitUntil(ctx, time.Second, 50*time.Millisecond, func() (string, bool, error) {
		return "", false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestWaitUntil_DoesNotOvershootTheTimeoutByAnInterval(t *testing.T) {
	start := time.Now()
	_, err := WaitUntil(context.Background(), 50*time.Millisecond, time.Second, func() (struct{}, bool, error) {
		return struct{}{}, false, nil
	})
	if err != ErrNotReady {
		t.Fatalf("err = %v, want ErrNotReady", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("WaitUntil took %v with a 50ms timeout; the final sleep must be capped to the time remaining, not the full interval", elapsed)
	}
}
