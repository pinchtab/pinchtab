package bridge

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestIsChromeProfileLockError(t *testing.T) {
	t.Parallel()

	msg := "chrome failed to start: [2046:2046:0309/221021.856597:ERROR:chrome/browser/process_singleton_posix.cc:363] The profile appears to be in use by another Chromium process"
	if !isChromeProfileLockError(msg) {
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

func TestClearStaleChromeProfileLockRemovesSingletonFiles(t *testing.T) {
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

	removed, err := clearStaleChromeProfileLock(profileDir, "")
	if err != nil {
		t.Fatalf("clearStaleChromeProfileLock() error = %v", err)
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

func TestClearStaleChromeProfileLockLeavesActiveProfileUntouched(t *testing.T) {
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

	removed, err := clearStaleChromeProfileLock(profileDir, "")
	if err != nil {
		t.Fatalf("clearStaleChromeProfileLock() error = %v", err)
	}
	if removed {
		t.Fatal("expected active profile lock to remain in place")
	}
	if _, err := os.Lstat(lockPath); err != nil {
		t.Fatalf("expected lock file to remain, got err=%v", err)
	}
}

func TestClearStaleChromeProfileLockFallsBackToPIDProbe(t *testing.T) {
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

	removed, err := clearStaleChromeProfileLock(profileDir, "another Chromium process (36)")
	if err != nil {
		t.Fatalf("clearStaleChromeProfileLock() error = %v", err)
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
