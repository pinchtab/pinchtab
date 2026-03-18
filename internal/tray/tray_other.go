//go:build !windows

package tray

// Run is a no-op on non-Windows platforms.
func Run(_ string, _ func(string) error, _ func()) {}

// Quit is a no-op on non-Windows platforms.
func Quit() {}

// IsAvailable returns false on non-Windows platforms.
func IsAvailable() bool {
	return false
}
