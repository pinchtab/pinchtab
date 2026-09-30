package bridge

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// EnsureChromiumSkeleton gives profileDir the Default/ directory and a seeded
// Preferences file Chromium expects; without Preferences a fresh profile is
// mistaken for a corrupt one. An existing Preferences file is never overwritten.
func EnsureChromiumSkeleton(profileDir string) error {
	defaultDir := filepath.Join(profileDir, "Default")
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(defaultDir, "Preferences"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString("{}"); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
