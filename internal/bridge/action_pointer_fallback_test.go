package bridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func takes(d time.Duration) func(context.Context, int64) error {
	return func(ctx context.Context, _ int64) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func hangs(ctx context.Context, _ int64) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestASlowClickSucceedsWhileTheActionHasTimeLeft(t *testing.T) {
	t.Parallel()
	const slow = 400 * time.Millisecond
	for _, tc := range []struct {
		name    string
		trusted func(context.Context, int64) error
		js      func(context.Context, int64) error
	}{
		{name: "slow trusted click, slow JS fallback", trusted: takes(slow), js: takes(slow)},
		{name: "stuck trusted click, slow JS fallback", trusted: hangs, js: takes(slow)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()

			if err := withTrustedFallback(ctx, 1, tc.trusted, tc.js); err != nil {
				t.Fatalf("withTrustedFallback() = %v, want the slow click to succeed", err)
			}
		})
	}
}

func TestAStuckTrustedClickFallsBackWithoutWaitingOnTheActionBudget(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	if err := withTrustedFallback(ctx, 1, hangs, takes(0)); err != nil {
		t.Fatalf("withTrustedFallback() = %v, want the JS fallback to land", err)
	}
	if elapsed := time.Since(start); elapsed > 4*trustedClickStepTimeout {
		t.Fatalf("click took %v, want the fallback within about %v", elapsed, trustedClickStepTimeout)
	}
}

func TestABlockedFallbackReturnsWellBeforeTheActionTimeout(t *testing.T) {
	t.Parallel()
	const actionTimeout = 4 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()

	start := time.Now()
	err := withTrustedFallback(ctx, 1, hangs, hangs)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("withTrustedFallback() = %v, want a step deadline", err)
	}
	if ctx.Err() != nil || elapsed > actionTimeout/2 {
		t.Fatalf("blocked fallback returned after %v, want well under the %v action timeout", elapsed, actionTimeout)
	}
}

func TestClickStepBudgetIsClamped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		timeout time.Duration
		want    time.Duration
	}{
		{timeout: 500 * time.Millisecond, want: minClickStepBudget},
		{timeout: 4 * time.Second, want: time.Second},
		{timeout: time.Minute, want: maxClickStepBudget},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
		got := clickStepBudget(ctx)
		cancel()
		if got > tc.want || got < tc.want-50*time.Millisecond {
			t.Errorf("timeout %v: budget %v, want about %v", tc.timeout, got, tc.want)
		}
	}
	if got := clickStepBudget(context.Background()); got != maxClickStepBudget {
		t.Errorf("no deadline: budget %v, want %v", got, maxClickStepBudget)
	}
}
