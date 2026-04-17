package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestChromeNeedsNoSandbox(t *testing.T) {
	origGOOS := runtimeGOOS
	origGeteuid := osGeteuid
	origMarker := containerMarkerPath
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osGeteuid = origGeteuid
		containerMarkerPath = origMarker
	})

	runtimeGOOS = "linux"
	osGeteuid = func() int { return 1000 }
	containerMarkerPath = t.TempDir() + "/missing-dockerenv"

	if chromeNeedsNoSandbox() {
		t.Fatal("expected no-sandbox compatibility to be disabled without root or container marker")
	}

	osGeteuid = func() int { return 0 }
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected root user to enable no-sandbox compatibility")
	}
	osGeteuid = func() int { return 1000 }

	containerMarkerPath = t.TempDir() + "/.dockerenv"
	if err := os.WriteFile(containerMarkerPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected container marker to enable no-sandbox compatibility")
	}
}

func TestShouldRetryChromeStartupWithDirectLaunch(t *testing.T) {
	canceledParent, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name      string
		parentCtx context.Context
		err       error
		want      bool
	}{
		{
			name:      "startup timeout retries",
			parentCtx: context.Background(),
			err:       context.DeadlineExceeded,
			want:      true,
		},
		{
			name:      "allocator context canceled retries",
			parentCtx: context.Background(),
			err:       context.Canceled,
			want:      true,
		},
		{
			name:      "wrapped context canceled retries",
			parentCtx: context.Background(),
			err:       fmt.Errorf("failed to start: %w", context.Canceled),
			want:      true,
		},
		{
			name:      "string matched context canceled retries",
			parentCtx: context.Background(),
			err:       errors.New("failed to connect to chrome: context canceled"),
			want:      true,
		},
		{
			name:      "parent cancellation does not retry",
			parentCtx: canceledParent,
			err:       context.Canceled,
			want:      false,
		},
		{
			name:      "other errors do not retry",
			parentCtx: context.Background(),
			err:       errors.New("exec format error"),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryChromeStartupWithDirectLaunch(tt.parentCtx, tt.err); got != tt.want {
				t.Fatalf("shouldRetryChromeStartupWithDirectLaunch() = %v, want %v", got, tt.want)
			}
		})
	}
}
