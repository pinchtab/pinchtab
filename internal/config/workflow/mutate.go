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

func CurrentConfigPath() string {
	configPath := os.Getenv("PINCHTAB_CONFIG")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}
	return configPath
}

func GetValue(path string) (string, error) {
	fc, _, err := config.LoadFileConfig()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	value, err := config.GetConfigValue(fc, path)
	if err != nil {
		return "", err
	}
	return value, nil
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

func ValidateCurrentFile() (string, []error, error) {
	configPath := CurrentConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return configPath, nil, fmt.Errorf("read config file: %w", err)
	}

	fc := &config.FileConfig{}
	if err := json.Unmarshal(data, fc); err != nil {
		return configPath, nil, fmt.Errorf("parse config: %w", err)
	}

	return configPath, config.ValidateFileConfig(fc), nil
}

func UpdateValue(path, value string) (*config.RuntimeConfig, bool, error) {
	change, err := PrepareSetValue(path, value)
	if err != nil {
		return nil, false, fmt.Errorf("set %s: %w", path, err)
	}
	if len(change.ValidationErrors) > 0 {
		return nil, false, change.ValidationErrors[0]
	}
	if err := SavePreparedChange(change); err != nil {
		return nil, false, err
	}
	return config.Load(), true, nil
}

func InitDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	fc := config.DefaultFileConfig()
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
