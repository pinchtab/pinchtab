//go:build !windows

package orchestrator

import (
	"os/exec"
	"syscall"
)

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

const sigTERM = syscall.SIGTERM

const sigKILL = syscall.SIGKILL
