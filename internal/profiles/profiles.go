package profiles

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

type ProfileManager struct {
	baseDir  string
	activity activity.Recorder
	mu       sync.RWMutex
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
	}
}

func (pm *ProfileManager) SetActivityRecorder(rec activity.Recorder) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.activity = rec
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
