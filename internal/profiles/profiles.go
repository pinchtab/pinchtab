package profiles

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/ids"
)

var idMgr = ids.NewManager()

var reservedWindowsProfileNames = map[string]struct{}{
	"CON":  {},
	"PRN":  {},
	"AUX":  {},
	"NUL":  {},
	"COM1": {},
	"COM2": {},
	"COM3": {},
	"COM4": {},
	"COM5": {},
	"COM6": {},
	"COM7": {},
	"COM8": {},
	"COM9": {},
	"LPT1": {},
	"LPT2": {},
	"LPT3": {},
	"LPT4": {},
	"LPT5": {},
	"LPT6": {},
	"LPT7": {},
	"LPT8": {},
	"LPT9": {},
}

func profileID(name string) string {
	return idMgr.ProfileID(name)
}

// ValidateProfileName enforces a cross-platform-safe profile name policy for
// filesystem usage and shell-adjacent process cleanup on Windows.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("profile name cannot be blank")
	}
	if trimmed != name {
		return fmt.Errorf("profile name cannot start or end with whitespace")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("profile name cannot contain '..'")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("profile name cannot contain '/' or '\\'")
	}
	if strings.HasSuffix(name, ".") {
		return fmt.Errorf("profile name cannot end with '.'")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("profile name contains invalid character %q", r)
	}
	base := name
	if dot := strings.IndexRune(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	if _, reserved := reservedWindowsProfileNames[strings.ToUpper(base)]; reserved {
		return fmt.Errorf("profile name cannot use reserved device name %q", base)
	}
	return nil
}

func isProfileNameValidationError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "profile name ")
}

type ProfileManager struct {
	baseDir string
	tracker *ActionTracker
	mu      sync.RWMutex
}

type ProfileMeta struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	UseWhen     string `json:"useWhen,omitempty"`
	Description string `json:"description,omitempty"`
}

type ProfileDetailedInfo struct {
	ID                string    `json:"id,omitempty"`
	Name              string    `json:"name"`
	Path              string    `json:"path"`
	CreatedAt         time.Time `json:"createdAt"`
	SizeMB            float64   `json:"sizeMB"`
	Source            string    `json:"source,omitempty"`
	ChromeProfileName string    `json:"chromeProfileName,omitempty"`
	AccountEmail      string    `json:"accountEmail,omitempty"`
	AccountName       string    `json:"accountName,omitempty"`
	HasAccount        bool      `json:"hasAccount,omitempty"`
	UseWhen           string    `json:"useWhen,omitempty"`
	Description       string    `json:"description,omitempty"`
}

func NewProfileManager(baseDir string) *ProfileManager {
	_ = os.MkdirAll(baseDir, 0755)
	return &ProfileManager{
		baseDir: baseDir,
		tracker: NewActionTracker(),
	}
}

func (pm *ProfileManager) findProfileDirByName(name string) (string, error) {
	direct := filepath.Join(pm.baseDir, name)
	if info, err := os.Stat(direct); err == nil && info.IsDir() {
		return direct, nil
	}

	entries, err := os.ReadDir(pm.baseDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pm.baseDir, entry.Name())
		if entry.Name() == profileID(name) {
			return dir, nil
		}
		meta := readProfileMeta(dir)
		if meta.Name == name {
			return dir, nil
		}
	}
	return "", fmt.Errorf("profile %q not found", name)
}

func (pm *ProfileManager) profileDir(name string) (string, error) {
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.findProfileDirByName(name)
}

func (pm *ProfileManager) Exists(name string) bool {
	_, err := pm.profileDir(name)
	return err == nil
}

func (pm *ProfileManager) ProfilePath(name string) (string, error) {
	return pm.profileDir(name)
}

func (pm *ProfileManager) List() ([]bridge.ProfileInfo, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entries, err := os.ReadDir(pm.baseDir)
	if err != nil {
		return nil, err
	}

	profiles := []bridge.ProfileInfo{}
	skip := map[string]bool{"bin": true, "profiles": true}
	for _, entry := range entries {
		if !entry.IsDir() || skip[entry.Name()] {
			continue
		}
		info, err := pm.profileInfo(entry.Name())
		if err != nil {
			continue
		}

		if _, err := os.Stat(filepath.Join(pm.baseDir, entry.Name(), "Default")); err != nil {
			continue
		}

		isTemporary := strings.HasPrefix(info.Name, "instance-")

		pathExists := true
		if _, err := os.Stat(info.Path); err != nil {
			pathExists = false
		}

		profiles = append(profiles, bridge.ProfileInfo{
			ID:                info.ID,
			Name:              info.Name,
			Path:              info.Path,
			PathExists:        pathExists,
			Created:           info.CreatedAt,
			Temporary:         isTemporary,
			DiskUsage:         int64(info.SizeMB * 1024 * 1024),
			Source:            info.Source,
			ChromeProfileName: info.ChromeProfileName,
			AccountEmail:      info.AccountEmail,
			AccountName:       info.AccountName,
			HasAccount:        info.HasAccount,
			UseWhen:           info.UseWhen,
			Description:       info.Description,
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func (pm *ProfileManager) profileInfo(dirName string) (ProfileDetailedInfo, error) {
	if err := ValidateProfileName(dirName); err != nil {
		return ProfileDetailedInfo{}, err
	}
	dir := filepath.Join(pm.baseDir, dirName)
	fi, err := os.Stat(dir)
	if err != nil {
		return ProfileDetailedInfo{}, err
	}

	size := dirSizeMB(dir)
	source := "created"
	if _, err := os.Stat(filepath.Join(dir, ".pinchtab-imported")); err == nil {
		source = "imported"
	}

	chromeProfileName, accountEmail, accountName, hasAccount := readChromeProfileIdentity(dir)
	meta := readProfileMeta(dir)
	profileName := meta.Name
	if profileName == "" {
		profileName = dirName
	}

	changed := false
	if meta.ID == "" {
		meta.ID = profileID(profileName)
		changed = true
	}
	if meta.Name == "" {
		meta.Name = profileName
		changed = true
	}
	if changed {
		_ = writeProfileMeta(dir, meta)
	}

	return ProfileDetailedInfo{
		ID:                meta.ID,
		Name:              profileName,
		Path:              dir,
		CreatedAt:         fi.ModTime(),
		SizeMB:            size,
		Source:            source,
		ChromeProfileName: chromeProfileName,
		AccountEmail:      accountEmail,
		AccountName:       accountName,
		HasAccount:        hasAccount,
		UseWhen:           meta.UseWhen,
		Description:       meta.Description,
	}, nil
}

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

func resolveImportSourcePath(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source path required")
	}

	cleaned := filepath.Clean(sourcePath)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("source path invalid: %w", err)
		}
		cleaned = abs
	}

	roots, err := allowedImportRoots()
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if pathWithinRoot(cleaned, root) {
			return cleaned, nil
		}
	}
	return "", fmt.Errorf("source path must be within %s", strings.Join(roots, " or "))
}

func allowedImportRoots() ([]string, error) {
	roots := []string{filepath.Clean(os.TempDir())}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	roots = append(roots, filepath.Clean(homeDir))
	return roots, nil
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")
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

func (pm *ProfileManager) RecordAction(profile string, record bridge.ActionRecord) {
	pm.tracker.Record(profile, record)
}

func (pm *ProfileManager) Logs(name string, limit int) []bridge.ActionRecord {
	return pm.tracker.GetLogs(name, limit)
}

func (pm *ProfileManager) Analytics(name string) bridge.AnalyticsReport {
	return pm.tracker.Analyze(name)
}

func dirSizeMB(path string) float64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return float64(total) / (1024 * 1024)
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

func (pm *ProfileManager) FindByID(id string) (string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entries, err := os.ReadDir(pm.baseDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pm.baseDir, entry.Name())
		meta := readProfileMeta(dir)
		if meta.ID == id {
			if meta.Name != "" {
				return meta.Name, nil
			}
			return entry.Name(), nil
		}
		if entry.Name() == id && meta.Name != "" {
			return meta.Name, nil
		}
		if meta.ID == "" && profileID(entry.Name()) == id {
			return entry.Name(), nil
		}
	}
	return "", fmt.Errorf("profile with id %q not found", id)
}
