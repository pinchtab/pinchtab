package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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

	return fc, configPath, nil
}

// SaveFileConfig saves a FileConfig to the specified path.
func SaveFileConfig(fc *FileConfig, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
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
