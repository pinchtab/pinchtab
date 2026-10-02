package devtools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const oversizedDuStub = "#!/bin/sh\nprintf '6000000\\t%s\\n' \"$2\"\n"

func runGoCacheTrim(t *testing.T, maxGB string) (repo, output string) {
	t.Helper()
	repo = t.TempDir()
	stubs := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "du"), []byte(oversizedDuStub), 0o755); err != nil { // #nosec G306 -- test stub must be executable.
		t.Fatal(err)
	}
	cache := filepath.Join(repo, ".gocache")
	for _, dir := range []string{filepath.Join(cache, "build", "00"), filepath.Join(cache, "ab"), filepath.Join(cache, "mod", "pkg")} {
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- test fixture.
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(cache, "mod", "pkg"), 0o555); err != nil { // #nosec G302 -- mirrors the read-only module cache.
		t.Fatal(err)
	}

	script := filepath.Join(repoRoot(t), "scripts", "lib", "gocache.sh")
	cmd := exec.Command("bash", "-c", `set -euo pipefail; source "$1"; use_repo_go_caches "$2"; trim_repo_go_cache "$2"`, "bash", script, repo) // #nosec G204 -- fixed script from this repo.
	cmd.Env = append(withoutKeys(os.Environ(), "GOCACHE", "GOTMPDIR"), "PATH="+stubs+":"+os.Getenv("PATH"), "PINCHTAB_GOCACHE_MAX_GB="+maxGB)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("trim_repo_go_cache failed: %v\n%s", err, out)
	}
	return repo, string(out)
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

func TestTrimRepoGoCacheEmptiesEveryEntryOverTheCap(t *testing.T) {
	repo, out := runGoCacheTrim(t, "")
	if !strings.Contains(out, "clearing") {
		t.Fatalf("an oversized cache must announce the trim, got:\n%s", out)
	}
	entries, err := os.ReadDir(filepath.Join(repo, ".gocache"))
	if err != nil {
		t.Fatalf("the trimmed cache dir must still exist: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("a trim must drop the cache below the cap, but %d entries survived (build/ and mod/ are invisible to go clean -cache)", len(entries))
	}
}

func TestTrimRepoGoCacheFallsBackOnAnInvalidCap(t *testing.T) {
	for _, maxGB := range []string{"2.5", "abc", "0", "-3"} {
		t.Run(maxGB, func(t *testing.T) {
			_, out := runGoCacheTrim(t, maxGB)
			if !strings.Contains(out, "using 5") {
				t.Fatalf("PINCHTAB_GOCACHE_MAX_GB=%s must warn and fall back to 5, got:\n%s", maxGB, out)
			}
		})
	}
}
