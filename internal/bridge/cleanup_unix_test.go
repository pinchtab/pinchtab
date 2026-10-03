//go:build !windows

package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

// cmdlineHasExactArg lives in cleanup.go, which is //go:build !windows. Keeping its
// test beside the tagged implementation is what stops `go vet ./...` and the package
// test binary from failing to build on Windows.
func TestCmdlineHasExactUserDataDirArgAvoidsPrefixCollision(t *testing.T) {
	args := [][]byte{
		[]byte("/Applications/Chrome"),
		[]byte("--user-data-dir=/var/lib/pt/profile10"),
		[]byte("--remote-debugging-port=9222"),
	}
	if cmdlineHasExactArg(args, "--user-data-dir=/var/lib/pt/profile1") {
		t.Fatal("profile1 matched profile10")
	}
	if !cmdlineHasExactArg(args, "--user-data-dir=/var/lib/pt/profile10") {
		t.Fatal("profile10 should match exact user-data-dir arg")
	}
}

func TestParentPIDFromProcStatSurvivesParensInComm(t *testing.T) {
	ppid, ok := parentPIDFromProcStat([]byte("4242 (chrome (v1) x) S 77 4242 4242 0 -1"))
	if !ok || ppid != 77 {
		t.Fatalf("ppid = %d, ok = %v, want 77", ppid, ok)
	}
	if _, ok := parentPIDFromProcStat([]byte("garbage")); ok {
		t.Fatal("stat without a comm field must not yield a parent")
	}
}

func TestCleanupOrphanedChromeProcesses_TempSweepSkipsDirsInUse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	peerDir := filepath.Join(tmp, "pinchtab-profile-peer")
	orphanDir := filepath.Join(tmp, "pinchtab-profile-orphan")
	lockedDir := filepath.Join(tmp, "pinchtab-profile-locked")
	for _, dir := range []string{peerDir, orphanDir, lockedDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	origOwned := isProfileOwnedByRunningPinchtabMock
	t.Cleanup(func() { isProfileOwnedByRunningPinchtabMock = origOwned })
	isProfileOwnedByRunningPinchtabMock = func(dir string) (bool, int) {
		return dir == lockedDir, fakePeerServer
	}
	killed := withProcessTree(t,
		map[int]int{
			fakeInit: 0, fakePeerServer: fakeInit, os.Getpid(): fakeInit,
			fakeChromeMain: fakePeerServer,
			fakeRenderer:   fakeInit,
		},
		map[string][]int{
			peerDir:   {fakeChromeMain},
			orphanDir: {fakeRenderer},
		})

	CleanupOrphanedChromeProcesses("")

	for _, dir := range []string{peerDir, lockedDir} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("temp profile still in use was removed: %s", dir)
		}
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Errorf("orphaned temp profile was kept: %s", orphanDir)
	}
	if len(*killed) != 1 || (*killed)[0] != fakeRenderer {
		t.Errorf("killed = %v, want the orphan %d only", *killed, fakeRenderer)
	}
}
