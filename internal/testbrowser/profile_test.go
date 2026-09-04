package testbrowser

import (
	"go/ast"
	"go/parser"
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

// A Chrome profile handed to t.TempDir is the defect itself: t.TempDir asserts its
// own RemoveAll succeeded, and a browser still flushing its cache makes that fail on
// an unrelated card's gate. The rule is checked over the whole module, because the
// next browser test is as likely to be written in a package that has none today.
// The subject is the tests themselves, so the enumeration comes from srccensus.TestTree
// rather than a walk written here: the nested-checkout exclusion is the part a bespoke walk
// silently loses, and it has one owner.
func TestNoBrowserProfileIsPointedAtATestTempDir(t *testing.T) {
	sites := 0

	for _, file := range srccensus.TestTree(t, "../..", moduleTestFileFloor) {
		found, violations := profileDirSites(t, file.Path, file.Text)
		sites += found
		for _, at := range violations {
			t.Errorf("%s hands a Chrome profile to t.TempDir at %s; use testbrowser.ProfileDir(t) — t.TempDir fails the test when the browser is still flushing its cache", file.Name, at)
		}
	}

	if sites < userDataDirSiteFloor {
		t.Fatalf("found only %d chromedp.UserDataDir call sites; the census matched almost nothing and would pass vacuously", sites)
	}
}

// profileDirSites counts the chromedp.UserDataDir calls in one file and names the ones whose
// directory comes from t.TempDir.
//
// The argument is resolved through a local variable, because most sites in the tree hold the
// path in one — they need it again for their own cleanup — so a rule reading only the
// argument expression is blind to the majority of the call sites it appears to police.
func profileDirSites(t *testing.T, path, src string) (int, []string) {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return 0, nil
	}

	whole := 0
	ast.Inspect(parsed, func(n ast.Node) bool {
		if isUserDataDirCall(n) != nil {
			whole++
		}
		return true
	})

	sites := 0
	var violations []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		locals := tempDirLocals(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call := isUserDataDirCall(n)
			if call == nil {
				return true
			}
			sites++
			if fromTempDir(call.Args[0], locals) {
				violations = append(violations, fset.Position(call.Pos()).String())
			}
			return true
		})
		return true
	})

	// Resolving a local means walking function bodies, which is where every site sits today.
	// A site outside one — a package-level option slice — would be counted by the whole-file
	// pass and silently skipped by this one, so the two counts must agree or the census is
	// quietly narrower than it reads.
	if sites != whole {
		t.Errorf("%s has %d chromedp.UserDataDir calls but only %d inside a function; a site outside a function body is not being checked", path, whole, sites)
	}
	return sites, violations
}

func isUserDataDirCall(n ast.Node) *ast.CallExpr {
	call, ok := n.(*ast.CallExpr)
	if !ok || !isSelectorCall(call.Fun, "chromedp", "UserDataDir") || len(call.Args) != 1 {
		return nil
	}
	return call
}

// tempDirLocals names the variables a function assigns from t.TempDir(), by := or by =.
func tempDirLocals(body *ast.BlockStmt) map[string]bool {
	locals := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		name, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		if fromTempDir(assign.Rhs[0], nil) {
			locals[name.Name] = true
		}
		return true
	})
	return locals
}

func fromTempDir(expr ast.Expr, locals map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		return isMethodCall(e.Fun, "TempDir")
	case *ast.Ident:
		return locals[e.Name]
	}
	return false
}

// The two spellings a browser test actually uses, pinned as a pair: the module walk can only
// report what the predicate recognises, so a rule reading the argument expression alone passes
// over every site that holds the path in a variable — which is most of them, and is how a real
// bare t.TempDir survived the sweep that was meant to remove them all.
func TestTheProfileCensusReadsBothSpellingsOfTheDirectory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		violation bool
	}{
		{name: "t.TempDir inline", body: `chromedp.UserDataDir(t.TempDir())`, violation: true},
		{name: "t.TempDir through a local", body: "profile := t.TempDir()\n\tchromedp.UserDataDir(profile)", violation: true},
		{name: "ProfileDir inline", body: `chromedp.UserDataDir(testbrowser.ProfileDir(t))`},
		{name: "ProfileDir through a local", body: "profile := testbrowser.ProfileDir(t)\n\tchromedp.UserDataDir(profile)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f(t *testing.T) {\n\t" + tc.body + "\n}\n"
			sites, violations := profileDirSites(t, "probe_test.go", src)
			if sites != 1 {
				t.Fatalf("counted %d UserDataDir sites in %q, want 1 — the census cannot report what it did not see", sites, tc.body)
			}
			if got := len(violations) > 0; got != tc.violation {
				t.Errorf("violation = %v, want %v for %q", got, tc.violation, tc.body)
			}
		})
	}
}

// Two floors, because they fail for different reasons: the TestTree floor catches a walk that
// stopped seeing files at all, and the site floor catches a rule that stopped matching what
// it polices. Both are set well under the real counts so ordinary growth or deletion does not
// trip them.
const (
	moduleTestFileFloor  = 200
	userDataDirSiteFloor = 20
)

func isSelectorCall(fun ast.Expr, pkg, name string) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

func isMethodCall(fun ast.Expr, name string) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}
