package profiles

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// preflightProfileDestination validates name, ensures no existing profile or
// destination directory collides, and returns the derived profile id and
// destination path. Callers must hold pm.mu. dirConflictMsg formats the
// directory-collision message (receiving the profile name); pass nil to reuse
// the name-collision wording.
func (pm *ProfileManager) preflightProfileDestination(name string, dirConflictMsg func(string) string) (id, dest string, err error) {
	if err := ValidateProfileName(name); err != nil {
		return "", "", err
	}
	if _, err := pm.findProfileDirByName(name); err == nil {
		return "", "", tagged(ErrProfileExists, fmt.Sprintf("profile %q already exists", name))
	}
	id = profileID(name)
	dest = filepath.Join(pm.baseDir, id)
	if _, err := os.Stat(dest); err == nil {
		if dirConflictMsg != nil {
			return "", "", tagged(ErrProfileDirExists, dirConflictMsg(name))
		}
		return "", "", tagged(ErrProfileExists, fmt.Sprintf("profile %q already exists", name))
	}
	return id, dest, nil
}

func (pm *ProfileManager) Import(name, sourcePath string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id, _, err := pm.preflightProfileDestination(name, nil)
	if err != nil {
		return err
	}

	source, err := openImportSource(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = source.root.Close() }()

	srcInfo, err := source.root.Lstat(source.relative)
	if err != nil {
		return fmt.Errorf("source path invalid: %w", err)
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source path must not be a symlink")
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source path must be a directory")
	}
	if _, err := source.root.Stat(source.child("Default")); err != nil {
		if _, err2 := source.root.Stat(source.child("Preferences")); err2 != nil {
			return fmt.Errorf("source doesn't look like a Chrome user data dir (no Default/ or Preferences found)")
		}
	}

	slog.Info("importing profile", "name", name, "source", source.displayPath)
	// Import is all-or-nothing. The rooted Mkdir claims the preflighted id
	// atomically, so anything under it is ours to remove on failure.
	baseRoot, err := os.OpenRoot(pm.baseDir)
	if err != nil {
		return fmt.Errorf("open profile root: %w", err)
	}
	defer func() { _ = baseRoot.Close() }()
	if err := baseRoot.Mkdir(id, srcInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("create profile destination: %w", err)
	}
	destRoot, err := baseRoot.OpenRoot(id)
	if err != nil {
		_ = baseRoot.RemoveAll(id)
		return fmt.Errorf("open profile destination: %w", err)
	}
	cleanup := func() {
		_ = destRoot.Close()
		_ = baseRoot.RemoveAll(id)
	}
	if err := copyDir(source, destRoot); err != nil {
		cleanup()
		return fmt.Errorf("copy failed: %w", err)
	}

	if err := destRoot.WriteFile(".pinchtab-imported", []byte(source.displayPath), 0600); err != nil {
		slog.Warn("failed to write import marker", "err", err)
	}
	if err := writeProfileMetaRoot(destRoot, ProfileMeta{
		ID:   id,
		Name: name,
	}); err != nil {
		cleanup()
		return err
	}
	if err := destRoot.Close(); err != nil {
		_ = baseRoot.RemoveAll(id)
		return fmt.Errorf("close profile destination: %w", err)
	}
	return nil
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
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id, dest, err := pm.preflightProfileDestination(name, nil)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dest, "Default"), 0755); err != nil {
		return err
	}
	return writeProfileMeta(dest, ProfileMeta{
		ID:   id,
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
	if holder, held := pm.profileHolder(profileID(name), dir); held {
		return tagged(ErrProfileInUse, fmt.Sprintf("profile %q is in use by %s; stop it before resetting", name, holder))
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
	_, err := pm.remove(name, false)
	return err
}

// ForceDelete skips the in-use refusal. profiles is a leaf and cannot stop
// instances, so a held profile is removed anyway and the holder is returned
// for the response to report as orphaned — a browser left running on a
// deleted directory must never be silent.
func (pm *ProfileManager) ForceDelete(name string) (orphanedInstance string, err error) {
	return pm.remove(name, true)
}

func (pm *ProfileManager) remove(name string, force bool) (string, error) {
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir, err := pm.findProfileDirByName(name)
	if err != nil {
		return "", err
	}
	holder, held := pm.profileHolder(profileID(name), dir)
	if held && !force {
		return "", tagged(ErrProfileInUse, fmt.Sprintf("profile %q is in use by %s; delete with force=true to remove it anyway", name, holder))
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if held {
		return holder, nil
	}
	return "", nil
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

	existing, err := readProfileMetaForUpdate(dir)
	if err != nil {
		return err
	}
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

	newID, newDir, err := pm.preflightProfileDestination(newName, func(n string) string {
		return fmt.Sprintf("profile directory for %q already exists", n)
	})
	if err != nil {
		return err
	}

	original := readProfileMeta(oldDir)
	meta := original
	meta.ID = newID
	meta.Name = newName
	if err := writeProfileMeta(oldDir, meta); err != nil {
		return fmt.Errorf("failed to update profile metadata: %w", err)
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		// Restore what was read, not a recomputed equivalent: an imported profile
		// can carry an id that is not profileID(oldName), and the listing asserts
		// identity on that id.
		_ = writeProfileMeta(oldDir, original)
		return fmt.Errorf("failed to rename profile directory: %w", err)
	}

	slog.Info("profile renamed", "from", oldName, "to", newName)
	return nil
}
