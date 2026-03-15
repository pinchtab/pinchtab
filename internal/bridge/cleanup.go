//go:build !windows

package bridge

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CleanupOrphanedChromeProcesses kills Chrome processes left behind by
// previous PinchTab runs and removes temporary profile directories.
// Call on startup before launching Chrome.
func CleanupOrphanedChromeProcesses(profileDir string) {
	// 1. Kill Chrome processes using the configured profile dir
	// (from a previous crashed run that didn't shut down cleanly)
	if profileDir != "" {
		killed := killChromeByProfileDir(profileDir)
		if killed > 0 {
			slog.Info("cleanup: killed orphaned chrome processes using profile", "path", profileDir, "count", killed)
		}
	}

	// 2. Find and clean up temp profile dirs from previous headless fallbacks
	tmpDir := os.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		slog.Debug("cleanup: cannot read temp dir", "path", tmpDir, "err", err)
		return
	}

	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "pinchtab-profile-") {
			continue
		}
		dir := filepath.Join(tmpDir, e.Name())
		killChromeByProfileDir(dir)
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("cleanup: failed to remove temp profile dir", "path", dir, "err", err)
		} else {
			slog.Info("cleanup: removed orphaned temp profile dir", "path", dir)
		}
	}
}

// findChromePIDsByProfileDir returns PIDs of Chrome processes using the given profile directory.
func findChromePIDsByProfileDir(profileDir string) []int {
	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	needle := fmt.Sprintf("--user-data-dir=%s", profileDir)
	lines := bytes.Split(out, []byte{'\n'})
	var pids []int

	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || !strings.Contains(line, needle) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

// killChromeByProfileDir finds Chrome processes using the given profile
// directory, sends SIGTERM, waits briefly, then SIGKILL any survivors.
// Returns the number of processes killed.
func killChromeByProfileDir(profileDir string) int {
	pids := findChromePIDsByProfileDir(profileDir)
	if len(pids) == 0 {
		return 0
	}

	// SIGTERM first
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}

	// Give Chrome 500ms to shut down gracefully
	time.Sleep(500 * time.Millisecond)

	// SIGKILL any survivors
	killed := 0
	for _, pid := range pids {
		if err := syscall.Kill(pid, 0); err != nil {
			// Process already dead
			killed++
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err == nil {
			slog.Info("cleanup: force-killed chrome process", "pid", pid)
		}
		killed++
	}

	return killed
}
