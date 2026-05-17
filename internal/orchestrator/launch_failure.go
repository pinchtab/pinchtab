package orchestrator

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"strings"
)

// LaunchFailureReason categorizes why a browser instance failed to launch.
type LaunchFailureReason string

const (
	ReasonStartupTimeout     LaunchFailureReason = "startup_timeout"
	ReasonProcessExited      LaunchFailureReason = "process_exited"
	ReasonBinaryMissing      LaunchFailureReason = "binary_missing"
	ReasonCDPConnectFail     LaunchFailureReason = "cdp_connect_fail"
	ReasonHealthCheckTimeout LaunchFailureReason = "health_check_timeout"
	ReasonUnknown            LaunchFailureReason = "unknown"
)

// ClassifyLaunchFailure maps a launch error to a LaunchFailureReason; nil input returns ReasonUnknown.
func ClassifyLaunchFailure(err error) LaunchFailureReason {
	if err == nil {
		return ReasonUnknown
	}

	// Typed checks first so they win over generic string matches.
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return ReasonBinaryMissing
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return ReasonBinaryMissing
	}
	if errors.Is(err, fs.ErrNotExist) {
		return ReasonBinaryMissing
	}

	msg := err.Error()

	if strings.Contains(msg, "binary not found") ||
		strings.Contains(msg, "chrome/chromium not found") {
		return ReasonBinaryMissing
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return ReasonStartupTimeout
	}
	if strings.Contains(msg, "chrome startup timeout") {
		return ReasonStartupTimeout
	}

	if strings.Contains(msg, "health check timeout") {
		return ReasonHealthCheckTimeout
	}

	if strings.Contains(msg, "process exited") {
		return ReasonProcessExited
	}

	if strings.Contains(msg, "failed to connect to chrome") ||
		strings.Contains(msg, "failed to connect/inject") ||
		strings.Contains(msg, "chrome devtools not ready") {
		return ReasonCDPConnectFail
	}

	return ReasonUnknown
}

// recoverableFallbackReasons trigger fallback retry; anything else (incl. ReasonUnknown) bails out.
var recoverableFallbackReasons = map[LaunchFailureReason]struct{}{
	ReasonStartupTimeout:     {},
	ReasonProcessExited:      {},
	ReasonBinaryMissing:      {},
	ReasonCDPConnectFail:     {},
	ReasonHealthCheckTimeout: {},
}

// IsRecoverable reports whether the reason should trigger the next fallback candidate.
func IsRecoverable(reason LaunchFailureReason) bool {
	_, ok := recoverableFallbackReasons[reason]
	return ok
}
