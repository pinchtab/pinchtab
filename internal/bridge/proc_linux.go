//go:build linux

package bridge

import (
	"os/exec"
	"syscall"
)

// setPdeathsig sets the parent death signal on Linux so Chrome dies
// when the Go process exits unexpectedly.
func setPdeathsig(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
