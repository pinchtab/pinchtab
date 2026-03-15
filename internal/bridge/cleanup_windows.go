//go:build windows

package bridge

// CleanupOrphanedChromeProcesses is a no-op on Windows.
func CleanupOrphanedChromeProcesses(_ string) {}

// killChromeByProfileDir is a no-op on Windows.
func killChromeByProfileDir(_ string) int { return 0 }
