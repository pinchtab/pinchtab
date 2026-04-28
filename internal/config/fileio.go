package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// tightenConfigPerms best-effort enforces 0700 on the config directory and
// 0600 on the config file. It is invoked from both the save path (to handle
// pre-existing loose perms left by older versions) and the load path (to
// proactively recover regardless of whether the user mutates anything).
// Failures are intentionally swallowed: chmod can fail on read-only FS,
// foreign-owned files, or filesystems that don't honor unix perms, none of
// which should block reading config.
func tightenConfigPerms(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().Perm() != 0600 {
		_ = os.Chmod(path, 0600)
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err == nil && fi.Mode().Perm() != 0700 {
		_ = os.Chmod(dir, 0700)
	}
}

// LoadFileConfig loads a FileConfig from the default or specified path.
// Returns the config and the path it was loaded from.
func LoadFileConfig() (*FileConfig, string, error) {
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileConfig{}, configPath, nil
		}
		return nil, configPath, fmt.Errorf("failed to read config file: %w", err)
	}

	tightenConfigPerms(configPath)

	if isLegacyConfig(data) {
		fc, err := loadLegacyFileConfig(data)
		return fc, configPath, err
	}

	defaults := DefaultFileConfig()
	defaults.ConfigVersion = ""
	fc := &defaults
	if err := json.Unmarshal(data, fc); err != nil {
		return nil, configPath, fmt.Errorf("failed to parse config: %w", err)
	}
	NormalizeFileConfigAliasesFromJSON(fc, data)

	return fc, configPath, nil
}

// SaveFileConfig saves a FileConfig to the specified path.
func SaveFileConfig(fc *FileConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("failed to set config directory permissions: %w", err)
	}

	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("failed to set config file permissions: %w", err)
	}

	return nil
}

func loadLegacyFileConfig(data []byte) (*FileConfig, error) {
	var lc legacyFileConfig
	if err := json.Unmarshal(data, &lc); err != nil {
		return nil, fmt.Errorf("failed to parse legacy config: %w", err)
	}

	defaults := DefaultFileConfig()
	fc := &defaults
	legacy := convertLegacyConfig(&lc)
	if legacy.Server.Port != "" {
		fc.Server.Port = legacy.Server.Port
	}
	if legacy.Server.Token != "" {
		fc.Server.Token = legacy.Server.Token
	}
	if legacy.Server.StateDir != "" {
		fc.Server.StateDir = legacy.Server.StateDir
	}
	if legacy.InstanceDefaults.Mode != "" {
		fc.InstanceDefaults.Mode = legacy.InstanceDefaults.Mode
	}
	if legacy.InstanceDefaults.NoRestore != nil {
		fc.InstanceDefaults.NoRestore = legacy.InstanceDefaults.NoRestore
	}
	if legacy.InstanceDefaults.MaxTabs != nil {
		fc.InstanceDefaults.MaxTabs = legacy.InstanceDefaults.MaxTabs
	}
	if legacy.Profiles.BaseDir != "" {
		fc.Profiles.BaseDir = legacy.Profiles.BaseDir
	}
	if legacy.Profiles.DefaultProfile != "" {
		fc.Profiles.DefaultProfile = legacy.Profiles.DefaultProfile
	}
	if legacy.Security.AllowEvaluate != nil {
		fc.Security.AllowEvaluate = legacy.Security.AllowEvaluate
	}
	if legacy.Security.AllowMacro != nil {
		fc.Security.AllowMacro = legacy.Security.AllowMacro
	}
	if legacy.Security.AllowScreencast != nil {
		fc.Security.AllowScreencast = legacy.Security.AllowScreencast
	}
	if legacy.Security.AllowDownload != nil {
		fc.Security.AllowDownload = legacy.Security.AllowDownload
	}
	if legacy.Security.AllowUpload != nil {
		fc.Security.AllowUpload = legacy.Security.AllowUpload
	}
	if legacy.Timeouts.ActionSec != 0 {
		fc.Timeouts.ActionSec = legacy.Timeouts.ActionSec
	}
	if legacy.Timeouts.NavigateSec != 0 {
		fc.Timeouts.NavigateSec = legacy.Timeouts.NavigateSec
	}
	if legacy.MultiInstance.InstancePortStart != nil {
		fc.MultiInstance.InstancePortStart = legacy.MultiInstance.InstancePortStart
	}
	if legacy.MultiInstance.InstancePortEnd != nil {
		fc.MultiInstance.InstancePortEnd = legacy.MultiInstance.InstancePortEnd
	}

	return fc, nil
}

func NormalizeFileConfigAliasesFromJSON(fc *FileConfig, data []byte) {
	if fc == nil {
		return
	}

	type rawIDPI struct {
		AllowedDomains *[]string `json:"allowedDomains"`
	}
	type rawSecurity struct {
		AllowedDomains *[]string `json:"allowedDomains"`
		IDPI           *rawIDPI  `json:"idpi"`
	}
	type rawConfig struct {
		Security *rawSecurity `json:"security"`
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil || raw.Security == nil {
		return
	}

	switch {
	case raw.Security.AllowedDomains != nil:
		fc.Security.AllowedDomains = append([]string(nil), (*raw.Security.AllowedDomains)...)
	case raw.Security.IDPI != nil && raw.Security.IDPI.AllowedDomains != nil:
		fc.Security.AllowedDomains = append([]string(nil), (*raw.Security.IDPI.AllowedDomains)...)
	}
}
