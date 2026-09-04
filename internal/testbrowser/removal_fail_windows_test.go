//go:build windows

package testbrowser

import (
	"os"
	"path/filepath"
	"testing"
)

// makeRemovalFail arranges for os.RemoveAll(dir) to fail, standing in for a
// browser still writing into its profile.
//
// The unix approach — clearing the parent's write bit — arranges nothing here.
// os.Chmod on Windows only toggles the read-only attribute, and RemoveAll
// clears that on its way through, so the directory came away cleanly and the
// test found itself asserting against a successful removal: it reported that
// "the unremovable dir vanished", which was exactly right.
//
// What Windows does enforce is sharing. A file held open cannot be unlinked,
// and its directory cannot be removed while it is there, which surfaces as
// "The process cannot access the file because it is being used by another
// process." That is also the closer analogue of the case this guards — a real
// Chrome still flushing its cache is holding handles, not permissions.
func makeRemovalFail(t *testing.T, _, dir string) {
	t.Helper()
	held, err := os.Open(filepath.Join(dir, "Default", "Cache", "Cache_Data", "index"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
}
