//go:build linux

package bridge

import (
	"os/exec"
	"syscall"
)

// configureChromeProcess sets up process group and parent death signal on Linux.
func configureChromeProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:  true,
		Pdeathsig: syscall.SIGKILL,
	}
}

// killProcessGroup kills the entire Chrome process group.
func killProcessGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
