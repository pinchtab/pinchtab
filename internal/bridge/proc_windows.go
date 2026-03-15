//go:build windows

package bridge

import "os/exec"

// configureChromeProcess is a no-op on Windows.
// Windows uses Job Objects for process group management, which chromedp
// doesn't support yet. Chrome processes may need manual cleanup.
func configureChromeProcess(_ *exec.Cmd) {}

// killProcessGroup is a no-op on Windows (no POSIX process groups).
func killProcessGroup(_ int) error { return nil }
