//go:build !windows

package orchestrator

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the child process in its own process group.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends a signal to the entire process group.
func killProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

// processAlive checks if a process is still running.
func processAlive(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

// sigTERM is the termination signal.
const sigTERM = syscall.SIGTERM

// sigKILL is the kill signal.
const sigKILL = syscall.SIGKILL
