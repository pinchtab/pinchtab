//go:build !windows

package bridge

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Call on startup before launching Chrome.
func CleanupOrphanedChromeProcesses(profileDir string) {
	// Kill Chrome left over from a previous crashed run that didn't shut down
	// cleanly.
	if profileDir != "" {
		killed := killChromeByProfileDir(profileDir)
		if killed > 0 {
			slog.Info("cleanup: killed orphaned chrome processes using profile", "path", profileDir, "count", killed)
		}
	}

	// Clean up temp profile dirs from previous headless fallbacks.
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

// Matches both persistent profiles (--user-data-dir=.../profiles/...) and
// temp profiles (--user-data-dir=.../pinchtab-profile-*). Called on server
// shutdown — one ps scan, immediate SIGKILL.
func KillAllPinchtabChrome() int {
	var pids []int
	tempPids := findPIDsByNeedle("pinchtab-profile")
	persistPids := findPIDsByNeedle(".pinchtab/profiles")

	seen := make(map[int]bool)
	for _, pid := range append(tempPids, persistPids...) {
		if !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}

	if len(pids) == 0 {
		return 0
	}

	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	slog.Info("shutdown: killed pinchtab chrome processes", "count", len(pids))
	return len(pids)
}

// Test seam: overridden to simulate process presence without /proc or ps.
var findChromePIDsByProfileDirFunc = findChromePIDsByProfileDir

func findChromePIDsByProfileDir(profileDir string) []int {
	if pids := findChromePIDsByUserDataDirViaProc(profileDir); pids != nil {
		return pids
	}
	return findChromePIDsByUserDataDirViaPS(profileDir)
}

// Tries /proc first (Linux, always available even in minimal containers),
// then falls back to ps (macOS, BSDs).
func findPIDsByNeedle(needle string) []int {
	if pids := findPIDsViaProc(needle); pids != nil {
		return pids
	}
	return findPIDsViaPS(needle)
}

// Returns nil (not empty slice) if /proc is not available.
func findPIDsViaProc(needle string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil // /proc not available (macOS, some BSDs)
	}

	var pids []int
	self := os.Getpid()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 || pid == self {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		// /proc/*/cmdline uses null bytes as separators
		args := string(bytes.ReplaceAll(cmdline, []byte{0}, []byte{' '}))
		if strings.Contains(args, needle) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func findChromePIDsByUserDataDirViaProc(profileDir string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil // /proc not available (macOS, some BSDs)
	}

	want := "--user-data-dir=" + profileDir
	var pids []int
	self := os.Getpid()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 || pid == self {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if cmdlineHasExactArg(bytes.Split(bytes.TrimRight(cmdline, "\x00"), []byte{0}), want) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func cmdlineHasExactArg(args [][]byte, want string) bool {
	for _, arg := range args {
		if string(arg) == want {
			return true
		}
	}
	return false
}

func findChromePIDsByUserDataDirViaPS(profileDir string) []int {
	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	pattern := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta("--user-data-dir="+profileDir) + `(?:\s|$)`)
	lines := bytes.Split(out, []byte{'\n'})
	var pids []int
	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || !pattern.MatchString(line) {
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

func findPIDsViaPS(needle string) []int {
	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

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

// Sends SIGTERM, waits briefly, then SIGKILL any survivors.
func killChromeByProfileDir(profileDir string) int {
	pids := findChromePIDsByProfileDirFunc(profileDir)
	if len(pids) == 0 {
		return 0
	}

	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}

	time.Sleep(500 * time.Millisecond)

	killed := 0
	for _, pid := range pids {
		if err := syscall.Kill(pid, 0); err != nil {
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

// Sends SIGTERM without escalating to SIGKILL.
func terminateChromeByProfileDir(profileDir string) int {
	pids := findChromePIDsByProfileDirFunc(profileDir)
	if len(pids) == 0 {
		return 0
	}
	for _, pid := range pids {
		if err := syscall.Kill(pid, syscall.SIGTERM); err == nil {
			slog.Info("cleanup: SIGTERM chrome process", "pid", pid)
		}
	}
	return len(pids)
}
