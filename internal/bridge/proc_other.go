//go:build darwin

package bridge

import "os/exec"

// configureBrowserProcess is a no-op on macOS.
// The browser inherits the parent's process group. The orchestrator kills the
// entire bridge group, and in standalone bridge mode, Cleanup() handles it.
func configureBrowserProcess(_ *exec.Cmd) {}
