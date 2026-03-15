package bridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestCleanup_RemovesTempProfileDir(t *testing.T) {
	tmpDir := t.TempDir()
	profileDir := filepath.Join(tmpDir, "pinchtab-profile-test")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}

	b := New(nil, nil, &config.RuntimeConfig{})
	b.tempProfileDir = profileDir

	b.Cleanup()

	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Errorf("expected temp profile dir to be removed, but it still exists")
	}
	if b.tempProfileDir != "" {
		t.Errorf("expected tempProfileDir to be cleared, got %q", b.tempProfileDir)
	}
}

func TestCleanup_NoTempDir(t *testing.T) {
	b := New(nil, nil, &config.RuntimeConfig{})
	// Should not panic with no temp dir
	b.Cleanup()
}
