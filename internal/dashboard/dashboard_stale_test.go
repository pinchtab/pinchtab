package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The binary serves whatever bundle was last copied into the embed directory,
// however old, and nothing else notices. This is the notice: the embedded stamp
// must be the hash of the source as it is now, or the dashboard under test is
// not the dashboard in the tree.
func TestEmbeddedDashboardBundleIsBuiltFromTheCurrentSource(t *testing.T) {
	stamp := BundleStamp()
	if stamp == BundleNotBuilt {
		t.Skip("no dashboard bundle embedded; the UI route serves its explicit not-built page")
	}
	stale, err := BundleStale(stamp, "../../dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		current, _ := SourceStamp("../../dashboard")
		t.Fatalf("embedded dashboard bundle was built from source %s but the tree is at %s; run ./dev build dashboard and rebuild the binary before trusting anything the dashboard shows", stamp, current)
	}
}

func writeFixtureDashboard(t *testing.T, appSource string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		bundleInputsFile: "# fixture\nsrc\npackage.json\nbun.lock\nvite.config.ts\n",
		"src/App.tsx":    appSource,
		"package.json":   `{"name":"fixture"}`,
		"bun.lock":       "lock",
		"vite.config.ts": "export default {}",
		"src/.DS_Store":  "editor droppings",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAStampFromOlderSourceIsStaleAndAMatchingOneIsNot(t *testing.T) {
	dir := writeFixtureDashboard(t, "export const App = () => 'v1'")
	stamp, err := SourceStamp(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stale, err := BundleStale(stamp, dir); err != nil || stale {
		t.Fatalf("a bundle built from the current source reads stale (err %v)", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "App.tsx"), []byte("export const App = () => 'v2'"), 0o644); err != nil {
		t.Fatal(err)
	}
	if stale, err := BundleStale(stamp, dir); err != nil || !stale {
		t.Fatalf("a bundle built before the source changed reads current (err %v)", err)
	}
	if stale, err := BundleStale(BundleNotBuilt, dir); err != nil || stale {
		t.Fatalf("an absent bundle must never read stale (err %v)", err)
	}
}

func TestTheStampIgnoresDotfilesAndDependsOnEveryDeclaredInput(t *testing.T) {
	dir := writeFixtureDashboard(t, "v1")
	base, _ := SourceStamp(dir)
	if err := os.WriteFile(filepath.Join(dir, "src", ".DS_Store"), []byte("different droppings"), 0o644); err != nil {
		t.Fatal(err)
	}
	if again, _ := SourceStamp(dir); again != base {
		t.Fatal("a dotfile changed the stamp, so it would differ between machines")
	}
	inputs, err := loadBundleInputs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) < 4 {
		t.Fatalf("fixture declares only %d inputs; too few to prove every input counts", len(inputs))
	}
	for _, input := range inputs {
		path := filepath.Join(dir, input)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, "changed.txt")
		}
		if err := os.WriteFile(path, []byte("changed "+input), 0o644); err != nil {
			t.Fatal(err)
		}
		next, err := SourceStamp(dir)
		if err != nil {
			t.Fatal(err)
		}
		if next == base {
			t.Fatalf("changing declared input %s did not change the stamp", input)
		}
		base = next
	}
}

// Two floors defend the empty-stamp hazard and each owns one shape. An empty or
// comment-only declaration is refused by the LOADER, on a message naming the
// declaration file; a declaration naming only absent paths passes the loader
// with a non-empty list and an empty file set and is refused by the WALK, on a
// message naming the directory and the paths that were missing. Deleting either
// floor reds its own case and only its own.
func TestTheLoaderRefusesAnEmptyDeclarationByName(t *testing.T) {
	dir := writeFixtureDashboard(t, "v1")
	if err := os.WriteFile(filepath.Join(dir, bundleInputsFile), []byte("# nothing declared\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SourceStamp(dir)
	if err == nil || !strings.Contains(err.Error(), bundleInputsFile) || !strings.Contains(err.Error(), "declares no inputs") {
		t.Fatalf("an empty declaration must be refused by the loader, naming %s; got %v", bundleInputsFile, err)
	}
	if err := os.Remove(filepath.Join(dir, bundleInputsFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := SourceStamp(dir); err == nil || !strings.Contains(err.Error(), bundleInputsFile) {
		t.Fatalf("a missing declaration must be refused naming %s; got %v", bundleInputsFile, err)
	}
}

func TestTheWalkRefusesADeclarationOfOnlyAbsentPathsByName(t *testing.T) {
	dir := writeFixtureDashboard(t, "v1")
	if err := os.WriteFile(filepath.Join(dir, bundleInputsFile), []byte("srx\npackage.jsn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SourceStamp(dir)
	if err == nil {
		t.Fatal("a declaration naming only absent paths produced a stamp of nothing")
	}
	for _, want := range []string{"no bundle inputs under " + dir, "srx", "package.jsn"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("walk floor error %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "declares no inputs") {
		t.Fatalf("the loader answered a case only the walk can see: %v", err)
	}
}

// The recorded decision on a typo beside real inputs: it is skipped, because the
// build script skips it too and the two stamps must agree. The stamp is the
// stamp of the inputs that exist.
func TestAMissingDeclaredPathBesideRealOnesIsSkippedToMatchTheBuildScript(t *testing.T) {
	dir := writeFixtureDashboard(t, "v1")
	honest, err := SourceStamp(dir)
	if err != nil {
		t.Fatal(err)
	}
	declared, err := os.ReadFile(filepath.Join(dir, bundleInputsFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, bundleInputsFile), append(declared, []byte("typo.config.ts\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	withTypo, err := SourceStamp(dir)
	if err != nil {
		t.Fatalf("a typo beside real inputs must be skipped as the build script skips it: %v", err)
	}
	if withTypo != honest {
		t.Fatalf("the skipped path changed the stamp: %s vs %s", withTypo, honest)
	}
}

// The declaration is read from the real tree by the same loader the stamp uses,
// so the shell and Go cannot disagree on which inputs exist to hash.
func TestTheRealDeclarationIsReadable(t *testing.T) {
	inputs, err := loadBundleInputs("../../dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) < 5 {
		t.Fatalf("dashboard/%s declares only %d inputs: %v", bundleInputsFile, len(inputs), inputs)
	}
}
