package chrome

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

func TestCDPReachableUsesDiscoveryWithoutABinaryOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("discovery fixture uses a shell script")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, BinaryNames()[0])
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	original := probeCDP
	var launched string
	probeCDP = func(_ context.Context, path string, _ []string, _ time.Duration) (CDPProbeResult, error) {
		launched = path
		return CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { probeCDP = original })

	result := cdpReachableCheck(context.Background(), &browsers.DoctorEnv{})
	if result.Status != browsers.DoctorPass {
		t.Fatalf("status = %v, want pass: %s", result.Status, result.Detail)
	}
	if launched != binary {
		t.Fatalf("launched %q, want discovered binary %q", launched, binary)
	}
}
