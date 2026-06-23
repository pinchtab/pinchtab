//go:build linux

package bridge

import (
	"os/exec"
	"syscall"
)

// configureBrowserProcess sets parent death signal on Linux so the browser dies
// when the Go process exits unexpectedly.
// Does NOT set Setpgid — the browser stays in the parent's process group so the
// orchestrator can kill the entire bridge group at once.
func configureBrowserProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
}
