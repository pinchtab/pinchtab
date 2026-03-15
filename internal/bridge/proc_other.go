//go:build !linux

package bridge

import "os/exec"

// setPdeathsig is a no-op on non-Linux platforms (macOS, Windows).
// macOS doesn't support Pdeathsig; process groups handle cleanup instead.
func setPdeathsig(_ *exec.Cmd) {}
