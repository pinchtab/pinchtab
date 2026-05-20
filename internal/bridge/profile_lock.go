// profile_lock.go handles stale Chrome profile lock recovery.
//
// When a container restarts (or Chrome crashes), Chrome's SingletonLock,
// SingletonSocket, and SingletonCookie files may be left behind in the profile
// directory. On next startup Chrome sees these and refuses to launch with
// "The profile appears to be in use by another Chromium process".
//
// This code detects that error, checks whether the owning process is actually
// still running (via PID probe and process listing), and removes the stale
// lock files if it's safe to do so. It retries Chrome startup once after
// clearing the locks.

package bridge

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var chromeProfileProcessLister = findChromeProfileProcesses
var chromePIDIsRunning = isChromePIDRunning
var killChromeProfileProcesses = killProcesses
var isProfileOwnedByRunningPinchtabMock = isProfileOwnedByRunningPinchtab
var isPinchTabProcessFunc = isPinchTabProcess

// lockStartupGrace is the window after a PID file is written during which the
// Chrome process check is skipped. AcquireProfileLock writes the PID before
// InitChrome launches the browser, so a second process must not treat the
// missing Chrome as a stale lock during this startup window.
var lockStartupGrace = 2 * time.Minute

var chromeSingletonFiles = []string{
	"SingletonLock",
	"SingletonSocket",
	"SingletonCookie",
}

var chromeProfileLockPIDPattern = regexp.MustCompile(`(?:Chromium|Chrome) process \((\d+)\)`)

type chromeProfileProcess struct {
	PID     string
	Command string
}

func isChromeProfileLockError(msg string) bool {
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "The profile appears to be in use by another Chromium process") ||
		strings.Contains(msg, "The profile appears to be in use by another Chrome process") ||
		strings.Contains(msg, "process_singleton")
}

func clearStaleChromeProfileLock(profileDir, errMsg string) (bool, error) {
	if strings.TrimSpace(profileDir) == "" {
		return false, nil
	}

	if pid, ok := extractChromeProfileLockPID(errMsg); ok {
		running, err := chromePIDIsRunning(pid)
		if err != nil {
			slog.Warn("failed to probe chrome lock pid; falling back to process listing", "profile", profileDir, "pid", pid, "err", err)
		} else if running {
			// If we find the PID from the error message is still running,
			// check if it's actually managed by another active PinchTab.
			if owned, ptPid := isProfileOwnedByRunningPinchtabMock(profileDir); owned {
				slog.Warn("chrome profile lock appears active and owned by another pinchtab; leaving singleton files in place", "profile", profileDir, "pid", pid, "pinchtab_pid", ptPid)
				return false, nil
			}
			slog.Warn("chrome profile lock appears active but pinchtab owner is dead; proceeding with stale cleanup", "profile", profileDir, "pid", pid)
		}
	}

	processes, err := chromeProfileProcessLister(profileDir)
	if err != nil {
		if _, ok := extractChromeProfileLockPID(errMsg); ok {
			slog.Warn("profile process listing unavailable; proceeding with stale lock cleanup based on lock pid", "profile", profileDir, "err", err)
		} else {
			return false, err
		}
	}
	if len(processes) > 0 {
		if owned, ptPid := isProfileOwnedByRunningPinchtabMock(profileDir); owned {
			pids := make([]string, 0, len(processes))
			for _, proc := range processes {
				pids = append(pids, proc.PID)
			}
			slog.Warn("chrome profile lock appears active and owned by another pinchtab; leaving singleton files in place", "profile", profileDir, "pids", strings.Join(pids, ","), "pinchtab_pid", ptPid)
			return false, nil
		}

		// If no other PinchTab owns this profile, we can safely kill the stale Chrome processes.
		slog.Warn("chrome profile lock appears active but no pinchtab owner found; killing stale processes", "profile", profileDir)
		if err := killChromeProfileProcesses(processes); err != nil {
			slog.Error("failed to kill stale chrome processes", "profile", profileDir, "err", err)
			return false, nil
		}
	}

	removed := false
	for _, name := range chromeSingletonFiles {
		path := filepath.Join(profileDir, name)
		if _, err := os.Lstat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, fmt.Errorf("inspect %s: %w", path, err)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("remove %s: %w", path, err)
		}
		removed = true
	}

	return removed, nil
}

// quarantineCorruptedProfile renames profileDir to "<profileDir>.quarantine-<ts>"
// and recreates an empty directory at the original path. Used to recover
// from silent CDP attach failures where CloakBrowser refuses to ingest
// existing profile state.
func quarantineCorruptedProfile(profileDir string) (string, error) {
	profileDir = strings.TrimSpace(profileDir)
	if profileDir == "" {
		return "", fmt.Errorf("empty profile dir")
	}
	if _, err := os.Stat(profileDir); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat profile dir: %w", err)
	}
	quarantinePath := fmt.Sprintf("%s.quarantine-%d", profileDir, time.Now().Unix())
	if err := os.Rename(profileDir, quarantinePath); err != nil {
		return "", fmt.Errorf("rename profile dir: %w", err)
	}
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return quarantinePath, fmt.Errorf("recreate profile dir: %w", err)
	}
	return quarantinePath, nil
}

func isProfileOwnedByRunningPinchtab(profileDir string) (bool, int) {
	pidFile := filepath.Join(profileDir, "pinchtab.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("failed to read pinchtab pid file", "path", pidFile, "err", err)
		}
		return false, 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		slog.Debug("failed to parse pinchtab pid file", "path", pidFile, "err", err)
		return false, 0
	}

	if pid == os.Getpid() {
		return false, pid // It's us
	}

	running, err := chromePIDIsRunning(pid)
	if err == nil && running {
		// Even if the PID is running, check if it's actually a pinchtab process
		// to handle PID reuse.
		if isPinchTabProcessFunc(pid) {
			// Skip the Chrome check while the PID file is fresh: AcquireProfileLock
			// writes the file before InitChrome launches the browser, so a second
			// process must not steal the lock during that startup window.
			if info, statErr := os.Stat(pidFile); statErr == nil && time.Since(info.ModTime()) < lockStartupGrace {
				slog.Debug("profile lock belongs to a recently started pinchtab; treating as active",
					"profile", profileDir, "pid", pid)
				return true, pid
			}
			processes, procErr := chromeProfileProcessLister(profileDir)
			if procErr == nil && len(processes) == 0 {
				slog.Debug("pinchtab pid file points to a running pinchtab but no chrome is using the profile; treating lock as stale",
					"profile", profileDir, "pid", pid)
				return false, 0
			}
			if procErr != nil {
				slog.Debug("unable to verify chrome ownership for running pinchtab pid; keeping profile locked",
					"profile", profileDir, "pid", pid, "err", procErr)
			}
			slog.Debug("profile is owned by another active pinchtab", "profile", profileDir, "pid", pid)
			return true, pid
		}
		slog.Debug("PID in lockfile is running but not a pinchtab process (PID reuse)", "profile", profileDir, "pid", pid)
	} else {
		slog.Debug("PID in lockfile is not running", "profile", profileDir, "pid", pid, "err", err)
	}

	return false, 0
}

func AcquireProfileLock(profileDir string) error {
	if profileDir == "" {
		return nil
	}
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("mkdir profile dir: %w", err)
	}

	if owned, pid := isProfileOwnedByRunningPinchtab(profileDir); owned {
		return fmt.Errorf("profile %s is already in use by pinchtab process %d", profileDir, pid)
	}

	pidFile := filepath.Join(profileDir, "pinchtab.pid")
	slog.Debug("acquiring profile lock", "profile", profileDir, "pid", os.Getpid())
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func extractChromeProfileLockPID(msg string) (int, bool) {
	if msg == "" {
		return 0, false
	}
	match := chromeProfileLockPIDPattern.FindStringSubmatch(msg)
	if len(match) != 2 {
		return 0, false
	}
	pid := 0
	for _, ch := range match[1] {
		pid = pid*10 + int(ch-'0')
	}
	if pid <= 0 {
		return 0, false
	}
	return pid, true
}

func findChromeProfileProcesses(profileDir string) ([]chromeProfileProcess, error) {
	if strings.TrimSpace(profileDir) == "" {
		return nil, nil
	}

	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list chrome processes: %w", err)
	}

	return parseChromeProfileProcesses(out, profileDir), nil
}

func parseChromeProfileProcesses(out []byte, profileDir string) []chromeProfileProcess {
	if len(out) == 0 || strings.TrimSpace(profileDir) == "" {
		return nil
	}

	needleEquals := "--user-data-dir=" + profileDir
	needleSpace := "--user-data-dir " + profileDir
	lines := bytes.Split(out, []byte{'\n'})
	processes := make([]chromeProfileProcess, 0)

	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || (!strings.Contains(line, needleEquals) && !strings.Contains(line, needleSpace)) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		processes = append(processes, chromeProfileProcess{
			PID:     fields[0],
			Command: strings.TrimSpace(strings.TrimPrefix(line, fields[0])),
		})
	}

	return processes
}
