package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RecordEncodeInterval is the poll cadence for WaitForRecordEncode.
const RecordEncodeInterval = time.Second

// RecordEncodeTimeout bounds how long WaitForRecordEncode waits for the server
// to finish encoding before giving up.
const RecordEncodeTimeout = 300 * time.Second

// WaitForRecordEncode polls the recording status (via fetchStatus, which returns
// the raw `{"state","error"}` body) until the encode reaches a terminal state
// ("finished" or "idle"). It returns nil once encoding is done, an error if the
// server reports an encode error, or ErrNotReady on timeout. Transport (HTTP
// client vs MCP client) is supplied by the caller so CLI and MCP share one
// timeout/terminal-state/error contract. A transient fetch or decode failure is
// treated as not-yet-ready and retried until the deadline.
func WaitForRecordEncode(ctx context.Context, fetchStatus func() ([]byte, error)) error {
	_, err := WaitUntil(ctx, RecordEncodeTimeout, RecordEncodeInterval, func() (struct{}, bool, error) {
		raw, fetchErr := fetchStatus()
		if fetchErr != nil {
			return struct{}{}, false, nil
		}
		var s struct {
			State string `json:"state"`
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &s) != nil {
			return struct{}{}, false, nil
		}
		switch s.State {
		case "finished", "idle":
			if s.Error != "" {
				return struct{}{}, false, fmt.Errorf("encode failed: %s", s.Error)
			}
			return struct{}{}, true, nil
		}
		return struct{}{}, false, nil
	})
	return err
}

// ErrNotReady is returned by WaitUntil when the deadline passes before probe
// reports ready. Callers wrap it with their own message.
var ErrNotReady = errors.New("not ready before deadline")

// WaitUntil polls probe until it reports ready (returns the value), returns an
// error (short-circuits), or the deadline/ctx expires. It is PROBE-FIRST: probe
// runs before the first sleep, and the deadline is checked at the top of the
// loop (so the final, post-deadline sleep does not trigger an extra probe) —
// matching the existing `for now < deadline { probe; sleep }` loops.
func WaitUntil[T any](ctx context.Context, timeout, interval time.Duration, probe func() (T, bool, error)) (T, error) {
	var zero T
	var result T
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := Poll(waitCtx, interval, func() (bool, error) {
		value, ready, err := probe()
		if ready {
			result = value
		}
		return ready, err
	})
	switch {
	case err == nil:
		return result, nil
	case ctx.Err() != nil:
		return zero, ctx.Err()
	case errors.Is(err, context.DeadlineExceeded):
		return zero, ErrNotReady
	default:
		return zero, err
	}
}

func Poll(ctx context.Context, interval time.Duration, check func() (bool, error)) error {
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
