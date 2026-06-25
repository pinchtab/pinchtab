package bridge

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestQuarantineCorruptedProfile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	profileDir := filepath.Join(tmp, "default")
	if err := os.MkdirAll(filepath.Join(profileDir, "Default"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	marker := filepath.Join(profileDir, "Default", "Preferences")
	if err := os.WriteFile(marker, []byte("original"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	quarantinePath, err := quarantineCorruptedProfile(profileDir)
	if err != nil {
		t.Fatalf("quarantineCorruptedProfile: %v", err)
	}
	if quarantinePath == "" {
		t.Fatal("expected non-empty quarantine path")
	}
	if !strings.HasPrefix(quarantinePath, profileDir+".quarantine-") {
		t.Fatalf("quarantine path %q should start with %q.quarantine-", quarantinePath, profileDir)
	}

	// Original marker must live in the quarantined copy, not the new empty dir.
	if _, err := os.Stat(filepath.Join(quarantinePath, "Default", "Preferences")); err != nil {
		t.Fatalf("expected marker preserved in quarantine: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected fresh profile dir empty, but original marker still present: err=%v", err)
	}

	// Fresh profile dir must exist and be writable.
	info, err := os.Stat(profileDir)
	if err != nil {
		t.Fatalf("expected recreated profile dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory at %s", profileDir)
	}
}

func TestQuarantineCorruptedProfile_MissingDir(t *testing.T) {
	t.Parallel()

	path, err := quarantineCorruptedProfile(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if path != "" {
		t.Fatalf("missing dir should return empty quarantine path, got %q", path)
	}
}

func TestQuarantineCorruptedProfile_EmptyPath(t *testing.T) {
	t.Parallel()

	if _, err := quarantineCorruptedProfile(""); err == nil {
		t.Fatal("expected error for empty profile dir")
	}
	if _, err := quarantineCorruptedProfile("   "); err == nil {
		t.Fatal("expected error for blank profile dir")
	}
}

func TestIsProfileLockError(t *testing.T) {
	t.Parallel()

	msg := "chrome failed to start: [2046:2046:0309/221021.856597:ERROR:chrome/browser/process_singleton_posix.cc:363] The profile appears to be in use by another Chromium process"
	if !isProfileLockError(msg) {
		t.Fatal("expected profile lock error to be detected")
	}
}

func TestParseChromeProfileProcesses(t *testing.T) {
	t.Parallel()

	profileDir := "/data/.config/pinchtab/profiles/default"
	out := []byte("  36 /usr/bin/chromium-browser --user-data-dir=/data/.config/pinchtab/profiles/default --remote-debugging-port=9222\n  99 /usr/bin/chromium-browser --user-data-dir=/tmp/other\n")

	got := parseChromeProfileProcesses(out, profileDir)
	want := []chromeProfileProcess{
		{
			PID:     "36",
			Command: "/usr/bin/chromium-browser --user-data-dir=/data/.config/pinchtab/profiles/default --remote-debugging-port=9222",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseChromeProfileProcesses() = %#v, want %#v", got, want)
	}
}

func TestExtractChromeProfileLockPID(t *testing.T) {
	t.Parallel()

	msg := "The profile appears to be in use by another Chromium process (36) on another computer"
	pid, ok := extractChromeProfileLockPID(msg)
	if !ok {
		t.Fatal("expected pid to be parsed from profile lock error")
	}
	if pid != 36 {
		t.Fatalf("extractChromeProfileLockPID() = %d, want 36", pid)
	}
}

func TestClearStaleProfileLocksRemovesSingletonFiles(t *testing.T) {
	profileDir := t.TempDir()
	for _, name := range chromeSingletonFiles {
		if err := os.WriteFile(filepath.Join(profileDir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	orig := chromeProfileProcessLister
	origPID := chromePIDIsRunning
	origMock := isProfileOwnedByRunningPinchtabMock
	chromeProfileProcessLister = func(string) ([]chromeProfileProcess, error) {
		return nil, nil
	}
	chromePIDIsRunning = func(int) (bool, error) {
		return false, nil
	}
	isProfileOwnedByRunningPinchtabMock = func(string) (bool, int) {
		return false, 0
	}
	t.Cleanup(func() {
		chromeProfileProcessLister = orig
		chromePIDIsRunning = origPID
		isProfileOwnedByRunningPinchtabMock = origMock
	})

	removed, err := clearStaleProfileLocks(profileDir, "")
	if err != nil {
		t.Fatalf("clearStaleProfileLocks() error = %v", err)
	}
	if !removed {
		t.Fatal("expected singleton files to be removed")
	}

	for _, name := range chromeSingletonFiles {
		if _, err := os.Lstat(filepath.Join(profileDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got err=%v", name, err)
		}
	}
}

func TestClearStaleProfileLocksLeavesActiveProfileUntouched(t *testing.T) {
	profileDir := t.TempDir()
	lockPath := filepath.Join(profileDir, chromeSingletonFiles[0])
	if err := os.WriteFile(lockPath, []byte("x"), 0644); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	orig := chromeProfileProcessLister
	origPID := chromePIDIsRunning
	origMock := isProfileOwnedByRunningPinchtabMock
	chromeProfileProcessLister = func(string) ([]chromeProfileProcess, error) {
		return []chromeProfileProcess{{PID: "36", Command: "/usr/bin/chromium-browser --user-data-dir=" + profileDir}}, nil
	}
	chromePIDIsRunning = func(int) (bool, error) {
		return false, nil
	}
	isProfileOwnedByRunningPinchtabMock = func(string) (bool, int) {
		return true, 1234
	}
	t.Cleanup(func() {
		chromeProfileProcessLister = orig
		chromePIDIsRunning = origPID
		isProfileOwnedByRunningPinchtabMock = origMock
	})

	removed, err := clearStaleProfileLocks(profileDir, "")
	if err != nil {
		t.Fatalf("clearStaleProfileLocks() error = %v", err)
	}
	if removed {
		t.Fatal("expected active profile lock to remain in place")
	}
	if _, err := os.Lstat(lockPath); err != nil {
		t.Fatalf("expected lock file to remain, got err=%v", err)
	}
}

func TestClearStaleProfileLocksFallsBackToPIDProbe(t *testing.T) {
	profileDir := t.TempDir()
	lockPath := filepath.Join(profileDir, chromeSingletonFiles[0])
	if err := os.WriteFile(lockPath, []byte("x"), 0644); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	orig := chromeProfileProcessLister
	origPID := chromePIDIsRunning
	origMock := isProfileOwnedByRunningPinchtabMock
	chromeProfileProcessLister = func(string) ([]chromeProfileProcess, error) {
		return nil, os.ErrPermission
	}
	chromePIDIsRunning = func(pid int) (bool, error) {
		if pid != 36 {
			t.Fatalf("unexpected pid probe: got %d, want 36", pid)
		}
		return false, nil
	}
	isProfileOwnedByRunningPinchtabMock = func(string) (bool, int) {
		return false, 0
	}
	t.Cleanup(func() {
		chromeProfileProcessLister = orig
		chromePIDIsRunning = origPID
		isProfileOwnedByRunningPinchtabMock = origMock
	})

	removed, err := clearStaleProfileLocks(profileDir, "another Chromium process (36)")
	if err != nil {
		t.Fatalf("clearStaleProfileLocks() error = %v", err)
	}
	if !removed {
		t.Fatal("expected singleton file to be removed after stale pid probe")
	}
	if _, err := os.Lstat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file to be removed, got err=%v", err)
	}
}

func TestIsProfileOwnedByRunningPinchtabTreatsPinchTabPIDWithoutChromeAsStale(t *testing.T) {
	profileDir := t.TempDir()
	pidFile := filepath.Join(profileDir, "pinchtab.pid")
	if err := os.WriteFile(pidFile, []byte("1234"), 0644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	// Backdate the PID file so it falls outside the startup grace window.
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(pidFile, old, old); err != nil {
		t.Fatalf("backdate pid file: %v", err)
	}

	origPID := chromePIDIsRunning
	origPinchTab := isPinchTabProcessFunc
	origLister := chromeProfileProcessLister
	t.Cleanup(func() {
		chromePIDIsRunning = origPID
		isPinchTabProcessFunc = origPinchTab
		chromeProfileProcessLister = origLister
	})

	chromePIDIsRunning = func(pid int) (bool, error) { return pid == 1234, nil }
	isPinchTabProcessFunc = func(pid int) bool { return pid == 1234 }
	chromeProfileProcessLister = func(path string) ([]chromeProfileProcess, error) { return nil, nil }

	owned, _ := isProfileOwnedByRunningPinchtab(profileDir)
	if owned {
		t.Fatal("expected stale pinchtab pid with no chrome to be treated as not owned")
	}
}

func TestIsProfileOwnedByRunningPinchtabKeepsLockDuringStartup(t *testing.T) {
	profileDir := t.TempDir()
	// PID file written just now — Chrome hasn't launched yet (startup window).
	if err := os.WriteFile(filepath.Join(profileDir, "pinchtab.pid"), []byte("1234"), 0644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	origPID := chromePIDIsRunning
	origPinchTab := isPinchTabProcessFunc
	origLister := chromeProfileProcessLister
	t.Cleanup(func() {
		chromePIDIsRunning = origPID
		isPinchTabProcessFunc = origPinchTab
		chromeProfileProcessLister = origLister
	})

	chromePIDIsRunning = func(pid int) (bool, error) { return pid == 1234, nil }
	isPinchTabProcessFunc = func(pid int) bool { return pid == 1234 }
	chromeProfileProcessLister = func(path string) ([]chromeProfileProcess, error) { return nil, nil }

	owned, pid := isProfileOwnedByRunningPinchtab(profileDir)
	if !owned || pid != 1234 {
		t.Fatalf("expected lock to be held during startup window, got owned=%v pid=%d", owned, pid)
	}
}

func TestIsProfileOwnedByRunningPinchtabKeepsLockWhenChromeUsesProfile(t *testing.T) {
	profileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(profileDir, "pinchtab.pid"), []byte("1234"), 0644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	origPID := chromePIDIsRunning
	origPinchTab := isPinchTabProcessFunc
	origLister := chromeProfileProcessLister
	t.Cleanup(func() {
		chromePIDIsRunning = origPID
		isPinchTabProcessFunc = origPinchTab
		chromeProfileProcessLister = origLister
	})

	chromePIDIsRunning = func(pid int) (bool, error) { return pid == 1234, nil }
	isPinchTabProcessFunc = func(pid int) bool { return pid == 1234 }
	chromeProfileProcessLister = func(path string) ([]chromeProfileProcess, error) {
		return []chromeProfileProcess{{PID: "99", Command: "/usr/bin/chromium --user-data-dir=" + path}}, nil
	}

	owned, pid := isProfileOwnedByRunningPinchtab(profileDir)
	if !owned || pid != 1234 {
		t.Fatalf("expected profile with active chrome to stay locked, got owned=%v pid=%d", owned, pid)
	}
}

// M11 regression: quarantine must wait for the dying browser to release the
// profile before renaming, proceed anyway after the bounded wait times out,
// and recreate the dir with 0700.
func TestQuarantineCorruptedProfile_WaitsForBrowserExit(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	oldFind := findChromePIDsByProfileDirFunc
	oldWait := quarantineExitWait
	oldPoll := chromeExitPollInterval
	quarantineExitWait = 20 * time.Millisecond
	chromeExitPollInterval = 5 * time.Millisecond
	calls := 0
	findChromePIDsByProfileDirFunc = func(dir string) []int {
		calls++
		if calls < 2 {
			return []int{4242} // first poll: browser still holds the profile
		}
		return nil
	}
	t.Cleanup(func() {
		findChromePIDsByProfileDirFunc = oldFind
		quarantineExitWait = oldWait
		chromeExitPollInterval = oldPoll
	})

	quarantinePath, err := quarantineCorruptedProfile(profileDir)
	if err != nil {
		t.Fatalf("quarantineCorruptedProfile: %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected quarantine to poll for browser exit, polls=%d", calls)
	}
	if quarantinePath == "" {
		t.Fatal("expected quarantine to proceed after the browser exited")
	}
	info, err := os.Stat(profileDir)
	if err != nil {
		t.Fatalf("recreated profile dir missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("recreated profile dir perms = %o, want 0700", perm)
	}
}

func TestQuarantineCorruptedProfile_ProceedsAfterWaitTimeout(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	oldFind := findChromePIDsByProfileDirFunc
	oldWait := quarantineExitWait
	oldPoll := chromeExitPollInterval
	quarantineExitWait = 10 * time.Millisecond
	chromeExitPollInterval = 2 * time.Millisecond
	findChromePIDsByProfileDirFunc = func(dir string) []int { return []int{4242} } // never exits
	t.Cleanup(func() {
		findChromePIDsByProfileDirFunc = oldFind
		quarantineExitWait = oldWait
		chromeExitPollInterval = oldPoll
	})

	quarantinePath, err := quarantineCorruptedProfile(profileDir)
	if err != nil {
		t.Fatalf("quarantine should proceed (with a warning) after the wait times out: %v", err)
	}
	if quarantinePath == "" {
		t.Fatal("expected a quarantine path despite the timed-out wait")
	}
}
