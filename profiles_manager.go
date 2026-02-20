package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// profileID generates a stable 12-char hex ID from the profile name.
func profileID(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:6])
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

type ProfileInfo struct {
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

func (pm *ProfileManager) List() ([]ProfileInfo, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entries, err := os.ReadDir(pm.baseDir)
	if err != nil {
		return nil, err
	}

	var profiles []ProfileInfo
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
		profiles = append(profiles, info)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func (pm *ProfileManager) profileInfo(name string) (ProfileInfo, error) {
	dir := filepath.Join(pm.baseDir, name)
	fi, err := os.Stat(dir)
	if err != nil {
		return ProfileInfo{}, err
	}

	size := dirSizeMB(dir)
	source := "created"
	if _, err := os.Stat(filepath.Join(dir, ".pinchtab-imported")); err == nil {
		source = "imported"
	}

	chromeProfileName, accountEmail, accountName, hasAccount := readChromeProfileIdentity(dir)
	meta := readProfileMeta(dir)

	// Backfill ID for profiles created before ID generation was added.
	if meta.ID == "" {
		meta.ID = profileID(name)
		_ = writeProfileMeta(dir, meta)
	}

	return ProfileInfo{
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

	slog.Info("importing profile", "name", name, "source", sourcePath)
	if err := exec.Command("cp", "-a", sourcePath, dest).Run(); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dest, ".pinchtab-imported"), []byte(sourcePath), 0644); err != nil {
		slog.Warn("failed to write import marker", "err", err)
	}
	return nil
}

// ImportWithMeta imports a profile with metadata
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
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dest := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	return os.MkdirAll(filepath.Join(dest, "Default"), 0755)
}

// CreateWithMeta creates a profile with metadata
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

func (pm *ProfileManager) Rename(oldName, newName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	oldDir := filepath.Join(pm.baseDir, oldName)
	newDir := filepath.Join(pm.baseDir, newName)
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", oldName)
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("profile %q already exists", newName)
	}
	return os.Rename(oldDir, newDir)
}

func (pm *ProfileManager) Delete(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}
	return os.RemoveAll(dir)
}

// RecordAction records an action for a profile (implements ProfileService).
func (pm *ProfileManager) RecordAction(profile string, record ActionRecord) {
	pm.tracker.Record(profile, record)
}

func (pm *ProfileManager) Logs(name string, limit int) []ActionRecord {
	return pm.tracker.GetLogs(name, limit)
}

func (pm *ProfileManager) Analytics(name string) AnalyticsReport {
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

func readChromeProfileIdentity(profileRoot string) (string, string, string, bool) {
	chromeProfileName, lsEmail, lsName, lsHas := readLocalStateIdentity(filepath.Join(profileRoot, "Local State"))
	prefsEmail, prefsName, prefsHas := readPreferencesIdentity(filepath.Join(profileRoot, "Default", "Preferences"))

	email := prefsEmail
	if email == "" {
		email = lsEmail
	}

	accountName := prefsName
	if accountName == "" {
		accountName = lsName
	}

	hasAccount := prefsHas || lsHas || email != ""
	return chromeProfileName, email, accountName, hasAccount
}

func readPreferencesIdentity(path string) (string, string, bool) {
	var prefs struct {
		AccountInfo []struct {
			Email    string `json:"email"`
			FullName string `json:"full_name"`
			GaiaName string `json:"gaia_name"`
			GaiaID   string `json:"gaia"`
		} `json:"account_info"`
	}
	if !readJSON(path, &prefs) {
		return "", "", false
	}

	for _, account := range prefs.AccountInfo {
		email := account.Email
		name := account.FullName
		if name == "" {
			name = account.GaiaName
		}
		if email != "" || account.GaiaID != "" || name != "" {
			return email, name, true
		}
	}

	return "", "", false
}

func readLocalStateIdentity(path string) (string, string, string, bool) {
	var state struct {
		Profile struct {
			InfoCache map[string]struct {
				Name                       string `json:"name"`
				UserName                   string `json:"user_name"`
				GaiaName                   string `json:"gaia_name"`
				GaiaID                     string `json:"gaia_id"`
				IsConsentedPrimaryAccount  bool   `json:"is_consented_primary_account"`
				HasConsentedPrimaryAccount bool   `json:"has_consented_primary_account"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if !readJSON(path, &state) || len(state.Profile.InfoCache) == 0 {
		return "", "", "", false
	}

	entry, ok := state.Profile.InfoCache["Default"]
	if !ok {
		for _, v := range state.Profile.InfoCache {
			entry = v
			break
		}
	}

	profileName := entry.Name
	email := entry.UserName
	accountName := entry.GaiaName
	hasAccount := email != "" || entry.GaiaID != "" || entry.IsConsentedPrimaryAccount || entry.HasConsentedPrimaryAccount
	return profileName, email, accountName, hasAccount
}

func readJSON(path string, out any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(data, out); err != nil {
		return false
	}
	return true
}

func readProfileMeta(profileDir string) ProfileMeta {
	var meta ProfileMeta
	readJSON(filepath.Join(profileDir, "profile.json"), &meta)
	return meta
}

func writeProfileMeta(profileDir string, meta ProfileMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(profileDir, "profile.json"), data, 0644)
}

// UpdateMeta updates the profile metadata (useWhen, description)
func (pm *ProfileManager) UpdateMeta(name string, meta map[string]string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}

	// Read existing metadata
	existing := readProfileMeta(dir)

	// Update fields if provided
	if useWhen, ok := meta["useWhen"]; ok {
		existing.UseWhen = useWhen
	}
	if description, ok := meta["description"]; ok {
		existing.Description = description
	}

	return writeProfileMeta(dir, existing)
}

// FindByID returns the profile name matching the given ID, or empty string if not found.
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
