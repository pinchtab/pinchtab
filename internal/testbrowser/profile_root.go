package testbrowser

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	profilePrefix   = "pinchtab-test-profile-"
	staleProfileAge = time.Hour
)

var sweepOnce sync.Once

func newProfileDir() (string, error) {
	root := profileRoot()
	sweepOnce.Do(func() { sweepStaleProfiles(root, time.Now().Add(-staleProfileAge)) })
	return os.MkdirTemp(root, profilePrefix)
}

func profileRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return os.TempDir()
	}
	repo, ok := repoRoot(wd)
	if !ok {
		return os.TempDir()
	}
	root := filepath.Join(repo, ".tmp", "test-profiles")
	if err := os.MkdirAll(root, 0o750); err != nil {
		return os.TempDir()
	}
	return root
}

func repoRoot(from string) (string, bool) {
	for dir := from; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		if filepath.Dir(dir) == dir {
			return "", false
		}
	}
}

// Staleness is the profile dir's own mtime, so the age must exceed the longest
// test process: a sweep from one package must not take a live browser's profile
// from a package still running beside it.
func sweepStaleProfiles(root string, cutoff time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), profilePrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}
