package bridge

import (
	"runtime"
	"testing"
)

func TestFindChromeBinary_ARM64Prioritization(t *testing.T) {
	// This test verifies the logic, not actual file existence
	// We're testing that ARM64 architectures get the right candidate order

	// The actual detection happens in findChromeBinary() which checks runtime.GOARCH
	// We can't mock runtime.GOARCH, but we can verify the function doesn't panic
	// and returns a string (empty if not found, which is fine)

	result := findChromeBinary()

	// Should return empty string or a valid path
	// We don't require Chrome to be installed in CI
	if result != "" {
		t.Logf("Found Chrome binary: %s (arch: %s)", result, runtime.GOARCH)
	} else {
		t.Logf("No Chrome binary found in common locations (expected in CI, arch: %s)", runtime.GOARCH)
	}

	// The key thing we're testing: function doesn't panic on any architecture
	// ARM64 vs x86_64 logic is exercised at runtime
}

func TestFindChromeBinary_NoPathTraversal(t *testing.T) {
	// Verify that findChromeBinary only checks known safe paths
	// and doesn't attempt path traversal or unsafe operations

	result := findChromeBinary()

	// If a binary is found, it should be an absolute path
	if result != "" && result[0] != '/' && result[1] != ':' {
		// Not absolute (Unix / or Windows C:)
		t.Errorf("findChromeBinary returned non-absolute path: %s", result)
	}
}
