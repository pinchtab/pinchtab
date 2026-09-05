package dashboard

import (
	"os"
	"path/filepath"
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

// An absent or empty declaration is an error, never an empty hash: an empty list
// would make every stamp equal and the staleness guard green for every tree.
func TestAMissingOrEmptyInputListIsAnErrorNotAnEmptyHash(t *testing.T) {
	dir := writeFixtureDashboard(t, "v1")
	if err := os.WriteFile(filepath.Join(dir, bundleInputsFile), []byte("# nothing declared\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SourceStamp(dir); err == nil {
		t.Fatal("an empty input list produced a stamp")
	}
	if err := os.Remove(filepath.Join(dir, bundleInputsFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := SourceStamp(dir); err == nil {
		t.Fatal("a missing input list produced a stamp")
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
