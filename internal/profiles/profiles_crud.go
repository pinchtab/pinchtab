package profiles

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

func (pm *ProfileManager) Import(name, sourcePath string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, err := pm.findProfileDirByName(name); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	dest := filepath.Join(pm.baseDir, profileID(name))
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}

	resolvedSourcePath, err := resolveImportSourcePath(sourcePath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(resolvedSourcePath, "Default")); err != nil {
		if _, err2 := os.Stat(filepath.Join(resolvedSourcePath, "Preferences")); err2 != nil {
			return fmt.Errorf("source doesn't look like a Chrome user data dir (no Default/ or Preferences found)")
		}
	}

	srcInfo, err := os.Lstat(resolvedSourcePath)
	if err != nil {
		return fmt.Errorf("source path invalid: %w", err)
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source path must not be a symlink")
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source path must be a directory")
	}

	slog.Info("importing profile", "name", name, "source", resolvedSourcePath)
	if err := copyDir(resolvedSourcePath, dest); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dest, ".pinchtab-imported"), []byte(resolvedSourcePath), 0600); err != nil {
		slog.Warn("failed to write import marker", "err", err)
	}
	return writeProfileMeta(dest, ProfileMeta{
		ID:   profileID(name),
		Name: name,
	})
}

func (pm *ProfileManager) ImportWithMeta(name, sourcePath string, meta ProfileMeta) error {
	if err := pm.Import(name, sourcePath); err != nil {
		return err
	}
	if meta.ID == "" {
		meta.ID = profileID(name)
	}
	if meta.Name == "" {
		meta.Name = name
	}
	dest := filepath.Join(pm.baseDir, profileID(name))
	return writeProfileMeta(dest, meta)
}

func (pm *ProfileManager) Create(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, err := pm.findProfileDirByName(name); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	dest := filepath.Join(pm.baseDir, profileID(name))
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	if err := os.MkdirAll(filepath.Join(dest, "Default"), 0755); err != nil {
		return err
	}
	return writeProfileMeta(dest, ProfileMeta{
		ID:   profileID(name),
		Name: name,
	})
}

func (pm *ProfileManager) CreateWithMeta(name string, meta ProfileMeta) error {
	if err := pm.Create(name); err != nil {
		return err
	}
	if meta.ID == "" {
		meta.ID = profileID(name)
	}
	if meta.Name == "" {
		meta.Name = name
	}
	dest := filepath.Join(pm.baseDir, profileID(name))
	return writeProfileMeta(dest, meta)
}

func (pm *ProfileManager) Reset(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir, err := pm.findProfileDirByName(name)
	if err != nil {
		return err
	}

	resetProfileDir(dir)
	slog.Info("profile reset", "name", name)
	return nil
}

func resetProfileDir(dir string) {
	nukeDirs := []string{
		"Default/Sessions",
		"Default/Session Storage",
		"Default/Cache",
		"Default/Code Cache",
		"Default/GPUCache",
		"Default/Service Worker",
		"Default/blob_storage",
		"ShaderCache",
		"GrShaderCache",
	}

	nukeFiles := []string{
		"Default/Cookies",
		"Default/Cookies-journal",
		"Default/History",
		"Default/History-journal",
		"Default/Visited Links",
	}

	for _, d := range nukeDirs {
		path := filepath.Join(dir, d)
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("reset: failed to remove dir", "path", path, "err", err)
		}
	}
	for _, f := range nukeFiles {
		_ = os.Remove(filepath.Join(dir, f))
	}
}

func (pm *ProfileManager) Delete(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir, err := pm.findProfileDirByName(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (pm *ProfileManager) UpdateMeta(name string, meta map[string]string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := ValidateProfileName(name); err != nil {
		return err
	}

	dir, err := pm.findProfileDirByName(name)
	if err != nil {
		return err
	}

	existing := readProfileMeta(dir)
	if existing.Name == "" {
		existing.Name = name
	}

	if useWhen, ok := meta["useWhen"]; ok {
		existing.UseWhen = useWhen
	}
	if description, ok := meta["description"]; ok {
		existing.Description = description
	}

	return writeProfileMeta(dir, existing)
}

func (pm *ProfileManager) Rename(oldName, newName string) error {
	if err := ValidateProfileName(oldName); err != nil {
		return err
	}
	if err := ValidateProfileName(newName); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	oldDir, err := pm.findProfileDirByName(oldName)
	if err != nil {
		return err
	}

	if _, err := pm.findProfileDirByName(newName); err == nil {
		return fmt.Errorf("profile %q already exists", newName)
	}

	newDir := filepath.Join(pm.baseDir, profileID(newName))
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("profile directory for %q already exists", newName)
	}

	meta := readProfileMeta(oldDir)
	meta.ID = profileID(newName)
	meta.Name = newName
	if err := writeProfileMeta(oldDir, meta); err != nil {
		return fmt.Errorf("failed to update profile metadata: %w", err)
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		meta.ID = profileID(oldName)
		meta.Name = oldName
		_ = writeProfileMeta(oldDir, meta)
		return fmt.Errorf("failed to rename profile directory: %w", err)
	}

	slog.Info("profile renamed", "from", oldName, "to", newName)
	return nil
}
