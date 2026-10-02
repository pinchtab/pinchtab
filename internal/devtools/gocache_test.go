package devtools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const duStub = "#!/bin/sh\nif [ -e \"$0.measured\" ]; then kb=$DU_AFTER_TRIM_KB; else kb=6000000; : > \"$0.measured\"; fi\nprintf '%s\\t%s\\n' \"$kb\" \"$2\"\n"

const staleAge = 3 * time.Hour

var goCacheFixture = []struct {
	path    string
	stale   bool
	trimmed bool
}{
	{"ab/stale-d", true, true},
	{"ab/fresh-d", false, false},
	{"build/00/stale-a", true, true},
	{"build/00/fresh-a", false, false},
	{"mod/pkg/stale-d", true, false},
	{"mod/cache/download/stale-d", true, false},
	{"trim.txt", true, false},
}

func runGoCacheTrim(t *testing.T, maxGB, afterTrimKB string) (cache, output string) {
	t.Helper()
	repo := t.TempDir()
	stubs := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "du"), []byte(duStub), 0o755); err != nil { // #nosec G306 -- test stub must be executable.
		t.Fatal(err)
	}
	cache = filepath.Join(repo, ".gocache")
	stale := time.Now().Add(-staleAge)
	for _, entry := range goCacheFixture {
		path := filepath.Join(cache, entry.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- test fixture.
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if entry.stale {
			if err := os.Chtimes(path, stale, stale); err != nil {
				t.Fatal(err)
			}
		}
	}
	modPkg := filepath.Join(cache, "mod", "pkg")
	if err := os.Chmod(modPkg, 0o555); err != nil { // #nosec G302 -- mirrors the read-only module cache.
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(modPkg, 0o755) }) // #nosec G302 -- lets TempDir cleanup remove the fixture.

	script := filepath.Join(repoRoot(t), "scripts", "lib", "gocache.sh")
	cmd := exec.Command("bash", "-c", `set -euo pipefail; source "$1"; use_repo_go_caches "$2"; trim_repo_go_cache "$2"`, "bash", script, repo) // #nosec G204 -- fixed script from this repo.
	cmd.Env = append(withoutKeys(os.Environ(), "GOCACHE", "GOTMPDIR"), "PATH="+stubs+":"+os.Getenv("PATH"), "PINCHTAB_GOCACHE_MAX_GB="+maxGB, "DU_AFTER_TRIM_KB="+afterTrimKB)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("trim_repo_go_cache failed: %v\n%s", err, out)
	}
	return cache, string(out)
}

func withoutKeys(env []string, keys ...string) []string {
	kept := env[:0:0]
	for _, kv := range env {
		drop := false
		for _, key := range keys {
			if strings.HasPrefix(kv, key+"=") {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, kv)
		}
	}
	return kept
}

func TestTrimRepoGoCacheDeletesOnlyIdleEntriesOverTheCap(t *testing.T) {
	cache, out := runGoCacheTrim(t, "", "1000")
	if !strings.Contains(out, "deleting entries idle") {
		t.Fatalf("an oversized cache must announce the trim, got:\n%s", out)
	}
	if strings.Contains(out, "still") {
		t.Fatalf("a trim that got under the cap must not warn, got:\n%s", out)
	}
	if info, err := os.Stat(cache); err != nil || !info.IsDir() {
		t.Fatalf("the trimmed cache dir must still exist: %v", err)
	}
	for _, entry := range goCacheFixture {
		_, err := os.Stat(filepath.Join(cache, entry.path))
		if entry.trimmed && !os.IsNotExist(err) {
			t.Errorf("%s is an entry idle for %s and must be deleted, stat err: %v", entry.path, staleAge, err)
		}
		if !entry.trimmed && err != nil {
			t.Errorf("%s must survive the trim (fresh, module cache, or not a cache entry): %v", entry.path, err)
		}
	}
}

func TestTrimRepoGoCacheWarnsWhenFreshEntriesKeepItOverTheCap(t *testing.T) {
	cache, out := runGoCacheTrim(t, "", "6000000")
	if !strings.Contains(out, "still 5859 MB (cap 5 GB)") {
		t.Fatalf("a cache still over the cap must warn with its size and the cap, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(cache, "ab", "fresh-d")); err != nil {
		t.Fatalf("staying over the cap must not fall back to clearing fresh entries: %v", err)
	}
}

func TestTrimRepoGoCacheFallsBackOnAnInvalidCap(t *testing.T) {
	for _, maxGB := range []string{"2.5", "abc", "0", "-3"} {
		t.Run(maxGB, func(t *testing.T) {
			_, out := runGoCacheTrim(t, maxGB, "1000")
			if !strings.Contains(out, "using 5") {
				t.Fatalf("PINCHTAB_GOCACHE_MAX_GB=%s must warn and fall back to 5, got:\n%s", maxGB, out)
			}
		})
	}
}
