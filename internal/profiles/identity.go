package profiles

import (
	"encoding/json"
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
