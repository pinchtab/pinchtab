//go:build windows

package bridge

// CleanupOrphanedChromeProcesses is a no-op on Windows.
// Windows process management requires different tooling (taskkill, Job Objects).
func CleanupOrphanedChromeProcesses(_ string) {}
