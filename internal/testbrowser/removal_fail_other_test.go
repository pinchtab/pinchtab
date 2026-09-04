//go:build !windows

package testbrowser

import (
	"os"
	"testing"
)

// makeRemovalFail arranges for os.RemoveAll(dir) to fail, standing in for a
// browser still writing into its profile.
//
// On unix it is the parent's write bit that governs whether an entry can be
// unlinked, so clearing it is enough and the directory below is untouched.
func makeRemovalFail(t *testing.T, parent, _ string) {
	t.Helper()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
}
