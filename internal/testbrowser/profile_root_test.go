package testbrowser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepRemovesAStaleProfileAndKeepsAFreshOne(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	stale := mkdirAged(t, root, profilePrefix+"stale", now.Add(-2*staleProfileAge), "SingletonLock")
	fresh := mkdirAged(t, root, profilePrefix+"fresh", now)
	unrelated := mkdirAged(t, root, "someone-elses-dir", now.Add(-2*staleProfileAge))

	sweepStaleProfiles(root, now.Add(-staleProfileAge))

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale profile survived the sweep (%v)", err)
	}
	for _, kept := range []string{fresh, unrelated} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("the sweep removed %s: %v", filepath.Base(kept), err)
		}
	}
}

func TestProfileDirLivesUnderTheRepo(t *testing.T) {
	want, err := filepath.Abs(filepath.Join("..", "..", ".tmp", "test-profiles"))
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Dir(ProfileDir(t)); got != want {
		t.Errorf("profile parent = %s, want %s", got, want)
	}
}

func TestRepoRootIsFoundFromANestedPackageDir(t *testing.T) {
	repo := t.TempDir()
	nested := filepath.Join(repo, "internal", "pkg")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module probe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := repoRoot(nested); !ok || got != repo {
		t.Errorf("repoRoot = %q, %v; want %q", got, ok, repo)
	}
}

func mkdirAged(t *testing.T, root, name string, mtime time.Time, files ...string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(dir, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return dir
}
