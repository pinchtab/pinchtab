//go:build !windows

package bridge

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func isChromePIDRunning(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}

func killProcesses(processes []chromeProfileProcess) error {
	for _, proc := range processes {
		var pid int
		if _, err := fmt.Sscanf(proc.PID, "%d", &pid); err != nil {
			continue
		}
		if pid <= 0 {
			continue
		}
		// Straight to SIGKILL: in this stale-recovery path, being aggressive
		// is better to ensure the next startup succeeds.
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func isPinchTabProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "args=") // #nosec G204 -- pid is an int, not user-controlled string
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	line := strings.ToLower(string(out))
	return strings.Contains(line, "pinchtab")
}
