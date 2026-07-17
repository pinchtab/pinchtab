package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadDefaultPath(t *testing.T) {
	m := newMockServer()
	defer m.close()

	f := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	captureStdout(t, func() {
		Upload(m.server.Client(), m.base(), "", []string{f}, "#file", "")
	})

	if m.lastPath != "/upload" {
		t.Fatalf("expected /upload, got %s", m.lastPath)
	}
}

func TestUploadTabScopedPath(t *testing.T) {
	m := newMockServer()
	defer m.close()

	f := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	captureStdout(t, func() {
		Upload(m.server.Client(), m.base(), "", []string{f}, "#file", "tab1")
	})

	if m.lastPath != "/tabs/tab1/upload" {
		t.Fatalf("expected /tabs/tab1/upload, got %s", m.lastPath)
	}
}
