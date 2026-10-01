package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pinchtab/pinchtab/internal/config"
)

type PreparedChange struct {
	FileConfig       *config.FileConfig
	ConfigPath       string
	ValidationErrors []error
}

func GetValue(path string) (string, error) {
	return config.EffectiveConfigValue(path)
}

func PrepareSetValue(path, value string) (*PreparedChange, error) {
	return prepareChange(func(fc *config.FileConfig) error {
		return config.SetConfigValue(fc, path, value)
	})
}

func PreparePatch(jsonPatch string) (*PreparedChange, error) {
	return prepareChange(func(fc *config.FileConfig) error {
		return config.PatchConfigJSON(fc, jsonPatch)
	})
}

func SavePreparedChange(change *PreparedChange) error {
	if change == nil || change.FileConfig == nil {
		return fmt.Errorf("prepared change is empty")
	}
	if err := config.SaveFileConfig(change.FileConfig, change.ConfigPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// ValidateCurrentFile returns the file's gating errors and its non-gating advisories
// separately, because only the first decides whether the file is valid.
func ValidateCurrentFile() (string, []error, []string, error) {
	configPath := config.ConfigFilePath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return configPath, nil, nil, fmt.Errorf("read config file: %w", err)
	}

	fc := &config.FileConfig{}
	if err := json.Unmarshal(data, fc); err != nil {
		return configPath, nil, nil, fmt.Errorf("parse config: %w", err)
	}

	advisories := append(config.RetiredFileConfigAdvisories(data), config.FileConfigAdvisories(fc)...)
	return configPath, config.ValidateFileConfig(fc), advisories, nil
}

func InitDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	fc := config.DefaultFileConfig()
	// EnsureFileToken directly, not ProvisionFileToken: config init materialises a
	// FRESH default file, so there is no operator-authored content to protect and
	// generating a token is the whole point. This is the one recorded exemption.
	if _, err := config.EnsureFileToken(&fc); err != nil {
		return fmt.Errorf("generate auth token: %w", err)
	}

	if err := config.SaveFileConfig(&fc, path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func prepareChange(mutate func(*config.FileConfig) error) (*PreparedChange, error) {
	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := mutate(fc); err != nil {
		return nil, err
	}
	return &PreparedChange{
		FileConfig:       fc,
		ConfigPath:       configPath,
		ValidationErrors: config.ValidateFileConfig(fc),
	}, nil
}
