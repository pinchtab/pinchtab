//go:build !windows

package browserprobe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersionTimeoutDefaultsToFiveSeconds(t *testing.T) {
	if VersionTimeout != 5*time.Second {
		t.Fatalf("VersionTimeout = %v, want the 5s production default", VersionTimeout)
	}
}

func TestRunVersionHonorsVersionTimeout(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "slowbrowser")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := VersionTimeout
	VersionTimeout = 100 * time.Millisecond
	t.Cleanup(func() { VersionTimeout = original })

	start := time.Now()
	_, err := RunVersion(context.Background(), binary)
	if err == nil {
		t.Fatal("RunVersion returned no error for a binary that outlives VersionTimeout")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("RunVersion took %v, want it cut off near VersionTimeout", elapsed)
	}
}
