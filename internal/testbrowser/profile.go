package testbrowser

import (
	"os"
	"testing"
	"time"
)

// removalAttempts and removalBackoff bound the retry: Chrome's flush finishes in
// milliseconds, so a couple of retries clear the common case without lengthening a
// suite that is not racing.
const (
	removalAttempts = 3
	removalBackoff  = 50 * time.Millisecond
)

// ProfileDir returns a fresh Chrome user-data-dir for a browser-backed test and
// removes it, best effort, when the test ends.
//
// It exists because t.TempDir CANNOT hold a browser profile. Chrome's shutdown is
// asynchronous: when the test body returns the process is often still flushing
// Default/Cache/Cache_Data, so files appear under the tree while t.TempDir's
// RemoveAll is walking it, RemoveAll fails on a directory that was empty when it
// was read, and t.TempDir turns that into a TEST FAILURE. The failure names a
// directory that is "not empty", says nothing about the behaviour under test, and
// lands on whichever browser test lost the race that run — so every reader spends a
// turn proving the red is not theirs.
//
// Removal failure here is logged, never asserted: a leaked temp profile is the
// lesser evil. This tolerance is scoped to browser profiles on purpose — tests that
// launch no browser keep strict t.TempDir cleanup, which is what catches their own
// cleanup bugs.
func ProfileDir(t testing.TB) string {
	t.Helper()
	dir, err := newProfileDir()
	if err != nil {
		t.Fatalf("create browser profile dir: %v", err)
	}
	t.Cleanup(func() { removeProfileDir(t, dir) })
	return dir
}

type logger interface {
	Logf(format string, args ...any)
}

func removeProfileDir(t logger, dir string) {
	var err error
	for attempt := range removalAttempts {
		if attempt > 0 {
			time.Sleep(removalBackoff)
		}
		if err = os.RemoveAll(dir); err == nil {
			return
		}
	}
	t.Logf("browser profile dir %s left behind (the browser is probably still flushing it): %v", dir, err)
}
