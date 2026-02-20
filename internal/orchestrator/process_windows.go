//go:build windows

package orchestrator

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcGroup is a no-op on Windows (no process groups via Setpgid).
func setProcGroup(cmd *exec.Cmd) {}

// killProcessGroup kills the process on Windows (no group kill support).
func killProcessGroup(pid int, _ syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// processAlive checks if a process is still running on Windows.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds; we try to signal 0 equivalent.
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

// sigTERM on Windows maps to a regular kill.
const sigTERM = syscall.Signal(0xf)

// sigKILL on Windows maps to a regular kill.
const sigKILL = syscall.Signal(0x9)
