package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"testing"
)

func TestClassifyLaunchFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want LaunchFailureReason
	}{
		{
			name: "nil returns unknown",
			err:  nil,
			want: ReasonUnknown,
		},
		{
			name: "wrapped DeadlineExceeded → startup_timeout",
			err:  fmt.Errorf("chrome startup timeout after 20s: %w", context.DeadlineExceeded),
			want: ReasonStartupTimeout,
		},
		{
			name: "bare DeadlineExceeded → startup_timeout",
			err:  context.DeadlineExceeded,
			want: ReasonStartupTimeout,
		},
		{
			name: "exec.Error → binary_missing",
			err:  &exec.Error{Name: "chrome", Err: exec.ErrNotFound},
			want: ReasonBinaryMissing,
		},
		{
			name: "fs.PathError on binary → binary_missing",
			err:  &fs.PathError{Op: "stat", Path: "/no/such/chrome", Err: fs.ErrNotExist},
			want: ReasonBinaryMissing,
		},
		{
			name: "fs.ErrNotExist wrapped → binary_missing",
			err:  fmt.Errorf("locating binary: %w", fs.ErrNotExist),
			want: ReasonBinaryMissing,
		},
		{
			name: "missingBrowserBinaryError message (chrome) → binary_missing",
			err:  errors.New("chrome/chromium not found: please install chrome or chromium, or set 'binary' in config.json"),
			want: ReasonBinaryMissing,
		},
		{
			name: "missingBrowserBinaryError message (cloak) → binary_missing",
			err:  errors.New("cloakbrowser binary not found: set browser.binary ..."),
			want: ReasonBinaryMissing,
		},
		{
			name: "process exited before health check → process_exited",
			err:  errors.New("process exited before health check: signal: killed"),
			want: ReasonProcessExited,
		},
		{
			name: "health check timeout → health_check_timeout",
			err:  errors.New("health check timeout after 45s (127.0.0.1:9876 -> connection refused)"),
			want: ReasonHealthCheckTimeout,
		},
		{
			name: "failed to connect to chrome → cdp_connect_fail",
			err:  errors.New("failed to connect to chrome: websocket dial: connection refused"),
			want: ReasonCDPConnectFail,
		},
		{
			name: "remote connect/inject → cdp_connect_fail",
			err:  errors.New("failed to connect/inject via remote: handshake failed"),
			want: ReasonCDPConnectFail,
		},
		{
			name: "devtools not ready → cdp_connect_fail",
			err:  errors.New("chrome devtools not ready on port 9222: timeout"),
			want: ReasonCDPConnectFail,
		},
		{
			name: "generic error → unknown",
			err:  errors.New("something else entirely went wrong"),
			want: ReasonUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyLaunchFailure(tc.err)
			if got != tc.want {
				t.Errorf("ClassifyLaunchFailure(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyLaunchFailure_AllReasonsCovered(t *testing.T) {
	reasons := map[LaunchFailureReason]bool{
		ReasonStartupTimeout:     false,
		ReasonProcessExited:      false,
		ReasonBinaryMissing:      false,
		ReasonCDPConnectFail:     false,
		ReasonHealthCheckTimeout: false,
		ReasonUnknown:            false,
	}
	samples := []error{
		nil,
		context.DeadlineExceeded,
		&exec.Error{Name: "chrome", Err: exec.ErrNotFound},
		errors.New("process exited before health check"),
		errors.New("health check timeout after 45s"),
		errors.New("failed to connect to chrome"),
		errors.New("something else"),
	}
	for _, s := range samples {
		reasons[ClassifyLaunchFailure(s)] = true
	}
	for r, hit := range reasons {
		if !hit {
			t.Errorf("reason %q not produced by any sample input", r)
		}
	}
}
