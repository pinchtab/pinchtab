package handlers

import (
	"context"
	"time"
)

// pollUntil runs check immediately and then every interval until it reports done,
// returns an error, or ctx is cancelled. check returns (done, err): a non-nil err
// aborts the poll and is returned; (true, nil) succeeds; (false, nil) keeps polling.
// The wait between checks is cancellable via ctx, so callers share one cancellation
// behavior instead of each rolling its own (some with non-cancellable time.Sleep).
func pollUntil(ctx context.Context, interval time.Duration, check func() (bool, error)) error {
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
