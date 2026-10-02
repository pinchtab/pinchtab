package testbrowser

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// The property the fix delivers, proven deterministically rather than sampled by
// re-running a suite: a browser profile whose removal FAILS must not fail the test.
// The subtest's own recorder stands in for a real Chrome still flushing its cache.
//
// How a removal is made to fail is per-platform, so makeRemovalFail owns it: unix
// clears the parent's write bit, Windows holds a handle open inside the profile.
// Both produce the class of error RemoveAll reports mid-flush.
func TestProfileDirCleanupToleratesAFailedRemoval(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "profile")
	if err := os.MkdirAll(filepath.Join(dir, "Default", "Cache", "Cache_Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Default", "Cache", "Cache_Data", "index"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	makeRemovalFail(t, parent, dir)

	recorder := &cleanupRecorder{TB: t}
	removeProfileDir(recorder, dir)

	if recorder.failed {
		t.Fatalf("a failed profile removal failed the test; that is the flake this helper exists to remove")
	}
	if !recorder.logged {
		t.Error("a failed profile removal was silent; the leak must be visible in the test log")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the unremovable dir vanished, so this test did not exercise a failed removal: %v", err)
	}
}

func TestProfileDirRemovesTheDirectoryOnTheHappyPath(t *testing.T) {
	var dir string
	t.Run("inner", func(inner *testing.T) {
		dir = ProfileDir(inner)
		if err := os.WriteFile(filepath.Join(dir, "SingletonLock"), []byte("x"), 0o644); err != nil {
			inner.Fatal(err)
		}
	})
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("profile dir %s survived the test that created it (%v); the tolerance is for a racing browser, not a licence to leak", dir, err)
	}
}

// cleanupRecorder captures what removeProfileDir did to the test instead of doing it,
// so "does not fail" is an assertion rather than the absence of one.
type cleanupRecorder struct {
	testing.TB
	failed bool
	logged bool
}

func (r *cleanupRecorder) Helper()               {}
func (r *cleanupRecorder) Logf(string, ...any)   { r.logged = true }
func (r *cleanupRecorder) Errorf(string, ...any) { r.failed = true }
func (r *cleanupRecorder) Fatalf(string, ...any) { r.failed = true }
func (r *cleanupRecorder) Error(...any)          { r.failed = true }
func (r *cleanupRecorder) Fatal(...any)          { r.failed = true }
func (r *cleanupRecorder) Cleanup(f func())      { f() }

// A test that builds its own allocator picks its own profile directory and launch wait,
// which is how a profile landed on t.TempDir and launches flaked at chromedp's 20s
// default. With testbrowser the only caller both have one owner. The rule is module-wide
// because the next browser test is as likely to land in a package that has none today.
func TestOnlyTestbrowserLaunchesATestChrome(t *testing.T) {
	for _, file := range srccensus.TestTree(t, "../..", moduleTestFileFloor) {
		for _, at := range ownLaunchSites(t, file.Path, file.Text) {
			t.Errorf("%s builds its own Chrome at %s; use testbrowser.Tab, or testbrowser.Launch(t, flags...) for a browser-wide flag", file.Name, at)
		}
	}
}

func ownLaunchSites(t testing.TB, path, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	parsed := srccensus.Parse(t, fset, path, src, 0)
	var sites []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && (isSelectorCall(call.Fun, "chromedp", "NewExecAllocator") || isSelectorCall(call.Fun, "chromedp", "UserDataDir")) {
			sites = append(sites, fset.Position(call.Pos()).String())
		}
		return true
	})
	return sites
}

func TestTheLaunchCensusRecognisesAnOwnAllocator(t *testing.T) {
	for _, tc := range []struct {
		body string
		own  bool
	}{
		{body: `chromedp.NewExecAllocator(ctx, opts...)`, own: true},
		{body: `chromedp.UserDataDir(dir)`, own: true},
		{body: `testbrowser.Launch(t, chromedp.Flag("ignore-certificate-errors", true))`},
		{body: `testbrowser.Tab(t)`},
	} {
		src := "package p\n\nfunc f(t *testing.T) {\n\t" + tc.body + "\n}\n"
		if got := len(ownLaunchSites(t, "probe_test.go", src)) > 0; got != tc.own {
			t.Errorf("own launch = %v, want %v for %q", got, tc.own, tc.body)
		}
	}
}

const moduleTestFileFloor = 200

func isSelectorCall(fun ast.Expr, pkg, name string) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}
