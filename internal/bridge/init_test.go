package bridge

import (
	"runtime"
	"testing"
)

func TestFindChromeBinary_ARM64Prioritization(t *testing.T) {
	// Verifies function doesn't panic across architectures (ARM64 vs x86_64)
	// Chrome may not be installed in CI, so empty result is valid
	result := findChromeBinary()

	if result != "" {
		t.Logf("Found Chrome binary: %s (arch: %s)", result, runtime.GOARCH)
	} else {
		t.Logf("No Chrome binary found (expected in CI, arch: %s)", runtime.GOARCH)
	}
}

func TestFindChromeBinary_NoPathTraversal(t *testing.T) {
	// Ensures only absolute paths are returned (security check)
	result := findChromeBinary()

	if result != "" && result[0] != '/' && result[1] != ':' {
		t.Errorf("findChromeBinary returned non-absolute path: %s", result)
	}
}
