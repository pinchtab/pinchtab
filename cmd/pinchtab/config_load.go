package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

func loadConfig() *config.RuntimeConfig {
	cfg, err := loadConfigWithMandatoryToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func loadLocalConfig() *config.RuntimeConfig {
	return config.Load()
}

func loadConfigWithMandatoryToken() (*config.RuntimeConfig, error) {
	if err := ensureMandatoryToken(); err != nil {
		return nil, err
	}
	return config.Load(), nil
}

func ensureMandatoryToken() error {
	if strings.TrimSpace(os.Getenv("PINCHTAB_TOKEN")) != "" {
		return nil
	}

	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return fmt.Errorf("load config file: %w", err)
	}
	if fc == nil {
		fc = &config.FileConfig{}
	}
	if strings.TrimSpace(fc.Server.Token) != "" {
		return nil
	}

	if strings.TrimSpace(os.Getenv("PINCHTAB_CONFIG")) != "" {
		if _, statErr := os.Stat(configPath); statErr == nil {
			return fmt.Errorf("server token is required in %s when PINCHTAB_CONFIG is set; add server.token or set PINCHTAB_TOKEN", configPath)
		} else if !os.IsNotExist(statErr) {
			return fmt.Errorf("stat config file: %w", statErr)
		}
	}

	changed, err := config.EnsureFileToken(fc)
	if err != nil {
		return fmt.Errorf("ensure server token: %w", err)
	}
	if !changed {
		return nil
	}

	if err := config.SaveFileConfig(fc, configPath); err != nil {
		return fmt.Errorf("save config file: %w", err)
	}
	return nil
}
