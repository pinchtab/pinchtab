package main

import (
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

type ProfileManager struct {
	baseDir string
	tracker *ActionTracker
	mu      sync.RWMutex
}

type ProfileInfo struct {
	Name              string    `json:"name"`
	Path              string    `json:"path"`
	CreatedAt         time.Time `json:"createdAt"`
	SizeMB            float64   `json:"sizeMB"`
	Source            string    `json:"source,omitempty"`
	ChromeProfileName string    `json:"chromeProfileName,omitempty"`
	AccountEmail      string    `json:"accountEmail,omitempty"`
	AccountName       string    `json:"accountName,omitempty"`
	HasAccount        bool      `json:"hasAccount,omitempty"`
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

	return ProfileInfo{
		Name:              name,
		Path:              dir,
		CreatedAt:         fi.ModTime(),
		SizeMB:            size,
		Source:            source,
		ChromeProfileName: chromeProfileName,
		AccountEmail:      accountEmail,
		AccountName:       accountName,
		HasAccount:        hasAccount,
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

func (pm *ProfileManager) Create(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dest := filepath.Join(pm.baseDir, name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	return os.MkdirAll(filepath.Join(dest, "Default"), 0755)
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
