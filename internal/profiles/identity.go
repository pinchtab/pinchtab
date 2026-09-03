package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

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

// readProfileMeta answers with the zero value when the metadata is absent or
// unreadable. That is right for a listing, which must still describe a profile
// whose sidecar is missing, and wrong for anything that writes the result back —
// see readProfileMetaForUpdate.
func readProfileMeta(profileDir string) ProfileMeta {
	var meta ProfileMeta
	readJSON(profileMetaPath(profileDir), &meta)
	return meta
}

// readProfileMetaForUpdate is readProfileMeta for a read-modify-write. It tells
// absent (nothing to preserve, start from zero) apart from present-but-unreadable,
// which the zero value cannot represent: a partial update that starts from zero
// writes back a record missing every field it failed to read, turning one bad read
// into permanent loss of user-authored text.
func readProfileMetaForUpdate(profileDir string) (ProfileMeta, error) {
	path := profileMetaPath(profileDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ProfileMeta{}, nil
	}
	if err != nil {
		return ProfileMeta{}, fmt.Errorf("read profile metadata: %w", err)
	}
	var meta ProfileMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return ProfileMeta{}, fmt.Errorf("profile metadata at %s is unreadable, so updating it would discard what it holds: %w", path, err)
	}
	return meta, nil
}

func profileMetaPath(profileDir string) string {
	return filepath.Join(profileDir, "profile.json")
}

// writeProfileMeta replaces the sidecar atomically. A truncated profile.json
// reads back as no metadata at all, which costs the profile its id and the
// operator's own useWhen/description text — the same reason the session stores
// in this repo write through a temp file and rename.
func writeProfileMeta(profileDir string, meta ProfileMeta) error {
	data, err := profileMetaJSON(meta)
	if err != nil {
		return err
	}
	path := profileMetaPath(profileDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func writeProfileMetaRoot(root *os.Root, meta ProfileMeta) error {
	data, err := profileMetaJSON(meta)
	if err != nil {
		return err
	}
	return root.WriteFile("profile.json", data, 0644)
}

func profileMetaJSON(meta ProfileMeta) ([]byte, error) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	return data, nil
}
