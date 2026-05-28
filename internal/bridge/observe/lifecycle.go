package observe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// WaitForQuietWindow blocks until either no Page.lifecycleEvent has been
// observed for `quiet` duration, or `ceiling` has elapsed since the call
// started. Returns the duration actually waited.
//
// The listener registered against ctx remains attached to the target after
// return — chromedp does not expose deregistration — but a `done` atomic
// short-circuits its body so post-return events are no-ops. Matches the
// pattern used by cdpops/navigation.go redirect tracking.
//
// quiet and ceiling clamp to non-zero minimums so callers cannot accidentally
// disable the wait by passing zero.
func WaitForQuietWindow(ctx context.Context, quiet, ceiling time.Duration) (time.Duration, error) {
	if quiet <= 0 {
		quiet = 250 * time.Millisecond
	}
	if ceiling <= 0 {
		ceiling = 750 * time.Millisecond
	}
	if ceiling < quiet {
		ceiling = quiet
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.SetLifecycleEventsEnabled(true).Do(ctx)
	})); err != nil {
		return 0, err
	}

	var (
		mu        sync.Mutex
		lastEvent = time.Now()
		done      atomic.Bool
	)

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if done.Load() {
			return
		}
		if _, ok := ev.(*page.EventLifecycleEvent); ok {
			mu.Lock()
			lastEvent = time.Now()
			mu.Unlock()
		}
	})

	start := time.Now()
	deadline := start.Add(ceiling)
	pollInterval := quiet / 4
	if pollInterval < 10*time.Millisecond {
		pollInterval = 10 * time.Millisecond
	}

	for {
		now := time.Now()
		if !now.Before(deadline) {
			done.Store(true)
			return now.Sub(start), nil
		}
		mu.Lock()
		sinceLast := now.Sub(lastEvent)
		mu.Unlock()
		if sinceLast >= quiet {
			done.Store(true)
			return now.Sub(start), nil
		}
		select {
		case <-ctx.Done():
			done.Store(true)
			return time.Since(start), ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
