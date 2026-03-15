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
)

// CleanupOrphanedTempProfiles finds and kills Chrome processes using
// pinchtab-profile-* temp directories, then removes those directories.
// Call on startup to clean up leftovers from previous crashed runs.
func CleanupOrphanedTempProfiles() {
	tmpDir := os.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		slog.Debug("cleanup: cannot read temp dir", "path", tmpDir, "err", err)
		return
	}

	var orphanDirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "pinchtab-profile-") {
			orphanDirs = append(orphanDirs, filepath.Join(tmpDir, e.Name()))
		}
	}

	if len(orphanDirs) == 0 {
		return
	}

	slog.Info("cleanup: found orphaned temp profile dirs", "count", len(orphanDirs))

	for _, dir := range orphanDirs {
		killChromeByProfileDir(dir)
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("cleanup: failed to remove temp profile dir", "path", dir, "err", err)
		} else {
			slog.Info("cleanup: removed orphaned temp profile dir", "path", dir)
		}
	}
}

// killChromeByProfileDir finds Chrome processes using the given profile
// directory and sends them SIGTERM.
func killChromeByProfileDir(profileDir string) {
	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return
	}

	needle := fmt.Sprintf("--user-data-dir=%s", profileDir)
	lines := bytes.Split(out, []byte{'\n'})

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

		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			slog.Debug("cleanup: failed to kill chrome process", "pid", pid, "err", err)
		} else {
			slog.Info("cleanup: killed orphaned chrome process", "pid", pid, "profileDir", profileDir)
		}
	}
}
