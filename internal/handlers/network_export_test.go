package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStaleTmpExports(t *testing.T) {
	stateDir := t.TempDir()
	exportDir := filepath.Join(stateDir, "exports")
	if err := os.MkdirAll(exportDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create a stale .tmp file (backdate mtime well past the 5-min threshold).
	stalePath := filepath.Join(exportDir, "network-old.har.tmp")
	if err := os.WriteFile(stalePath, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-10 * time.Minute)
	os.Chtimes(stalePath, staleTime, staleTime)

	// Create a fresh .tmp file that should be kept (could be in-flight).
	freshPath := filepath.Join(exportDir, "network-new.ndjson.tmp")
	if err := os.WriteFile(freshPath, []byte("fresh"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create a regular completed file that should never be touched.
	completedPath := filepath.Join(exportDir, "session.har")
	if err := os.WriteFile(completedPath, []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}

	CleanupStaleTmpExports(stateDir)

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Error("stale .tmp file should have been removed")
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Error("fresh .tmp file should have been kept")
	}
	if _, err := os.Stat(completedPath); err != nil {
		t.Error("completed .har file should have been kept")
	}
}

func TestCleanupStaleTmpExports_NoDir(t *testing.T) {
	// Should not panic when exports/ doesn't exist.
	CleanupStaleTmpExports(t.TempDir())
}
