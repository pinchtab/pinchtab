//go:build darwin

package bridge

import (
	"os/exec"
	"syscall"
)

// configureChromeProcess sets up process group on macOS.
// macOS doesn't support Pdeathsig; process groups handle cleanup instead.
func configureChromeProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire Chrome process group.
func killProcessGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
