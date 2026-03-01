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

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/idutil"
)

var idMgr = idutil.NewManager()

// profileID generates a stable hash-based ID from profile name
// Returns format: prof_XXXXXXXX
func profileID(name string) string {
	return idMgr.ProfileID(name)
}

// validateProfileName checks for path traversal and other unsafe patterns
// Prevents directory traversal attacks (../, ..\, /, \)
func validateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("profile name cannot contain '..'")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("profile name cannot contain '/' or '\\'")
	}
	return nil
}

type ProfileManager struct {
	baseDir string
	tracker *ActionTracker
	mu      sync.RWMutex
}

type ProfileMeta struct {
	ID          string `json:"id,omitempty"`
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

func (pm *ProfileManager) Exists(name string) bool {
	if err := validateProfileName(name); err != nil {
		return false
	}
	dir := filepath.Join(pm.baseDir, name)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
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

		// Mark as temporary if it's an auto-generated instance profile
		isTemporary := strings.HasPrefix(entry.Name(), "instance-")

		// Check if path exists
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

func (pm *ProfileManager) profileInfo(name string) (ProfileDetailedInfo, error) {
	if err := validateProfileName(name); err != nil {
		return ProfileDetailedInfo{}, err
	}
	dir := filepath.Join(pm.baseDir, name)
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

	if meta.ID == "" {
		meta.ID = profileID(name)
		_ = writeProfileMeta(dir, meta)
	}

	return ProfileDetailedInfo{
		ID:                meta.ID,
		Name:              name,
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
	if err := validateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dest := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}

	if _, err := os.Stat(filepath.Join(sourcePath, "Default")); err != nil {
		if _, err2 := os.Stat(filepath.Join(sourcePath, "Preferences")); err2 != nil {
			return fmt.Errorf("source doesn't look like a Chrome user data dir (no Default/ or Preferences found)")
		}
	}

	// Validate source path exists and is a directory
	srcInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("source path invalid: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source path must be a directory")
	}

	slog.Info("importing profile", "name", name, "source", sourcePath)
	if err := copyDir(sourcePath, dest); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dest, ".pinchtab-imported"), []byte(sourcePath), 0600); err != nil {
		slog.Warn("failed to write import marker", "err", err)
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
	dest := filepath.Join(pm.baseDir, name)
	return writeProfileMeta(dest, meta)
}

func (pm *ProfileManager) Create(name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dest := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	return os.MkdirAll(filepath.Join(dest, "Default"), 0755)
}

func (pm *ProfileManager) CreateWithMeta(name string, meta ProfileMeta) error {
	if err := pm.Create(name); err != nil {
		return err
	}
	if meta.ID == "" {
		meta.ID = profileID(name)
	}
	dest := filepath.Join(pm.baseDir, name)
	return writeProfileMeta(dest, meta)
}

func (pm *ProfileManager) Reset(name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}

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

	slog.Info("profile reset", "name", name)
	return nil
}

func (pm *ProfileManager) Delete(name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
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

	dir := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}

	existing := readProfileMeta(dir)

	if useWhen, ok := meta["useWhen"]; ok {
		existing.UseWhen = useWhen
	}
	if description, ok := meta["description"]; ok {
		existing.Description = description
	}

	return writeProfileMeta(dir, existing)
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
		meta := readProfileMeta(filepath.Join(pm.baseDir, entry.Name()))
		if meta.ID == id {
			return entry.Name(), nil
		}
	}
	return "", fmt.Errorf("profile with id %q not found", id)
}
