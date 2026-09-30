package handlers

import (
	"context"
	"errors"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

const modalScopeAttempts = 2

var errModalUnstable = errors.New("topmost dialog changed twice")

type modalScopeReadError struct {
	stage   string
	recheck bool
	err     error
}

func (e *modalScopeReadError) Error() string {
	if e.recheck {
		return e.stage + ": " + e.err.Error()
	}
	return e.err.Error()
}

func (e *modalScopeReadError) Unwrap() error { return e.err }

// fn's error counts only from an attempt whose dialog re-read matched; an unstable attempt's error is discarded.
func withStableModalScope(ctx context.Context, frameID string, skip bool, fn func(modalNodeID int64, modalOpen bool) error) (workErr, scopeErr error) {
	if skip {
		return fn(0, false), nil
	}
	for range modalScopeAttempts {
		modalNodeID, modalOpen, err := bridge.TopmostModalNodeID(ctx, frameID)
		if err != nil {
			return nil, &modalScopeReadError{stage: "topmost dialog", err: err}
		}
		workErr = fn(modalNodeID, modalOpen)
		afterNodeID, afterOpen, err := bridge.TopmostModalNodeID(ctx, frameID)
		if err != nil {
			return nil, &modalScopeReadError{stage: "recheck topmost dialog", recheck: true, err: err}
		}
		if modalNodeID == afterNodeID && modalOpen == afterOpen {
			return workErr, nil
		}
	}
	return nil, errModalUnstable
}
