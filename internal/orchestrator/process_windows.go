//go:build windows

package orchestrator

import (
	"os"
	"os/exec"
	"syscall"
)

func setProcGroup(cmd *exec.Cmd) {}

func killProcessGroup(pid int, _ syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds; we try to signal 0 equivalent.
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

const sigTERM = syscall.Signal(0xf)

const sigKILL = syscall.Signal(0x9)
