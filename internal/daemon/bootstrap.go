package daemon

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

type ConfigBootstrapStatus string

const (
	ConfigCreated   ConfigBootstrapStatus = "created"
	ConfigRecovered ConfigBootstrapStatus = "recovered"
	ConfigVerified  ConfigBootstrapStatus = "verified"
)

func EnsureConfig(force bool) (string, *config.FileConfig, ConfigBootstrapStatus, error) {
	_, configPath, err := config.LoadFileConfig()
	if err != nil {
		return "", nil, "", err
	}

	exists := fileExists(configPath)
	if !exists || force {
		defaults := config.DefaultFileConfig()
		defaults.ConfigVersion = ""
		token, err := config.GenerateAuthToken()
		if err != nil {
			return "", nil, "", err
		}
		defaults.Server.Token = token
		if err := config.SaveFileConfig(&defaults, configPath); err != nil {
			return "", nil, "", err
		}
		status := ConfigCreated
		if exists {
			status = ConfigRecovered
		}
		return configPath, &defaults, status, nil
	}

	fileCfg, _, _ := config.LoadFileConfig()
	if fileCfg == nil {
		return configPath, nil, "", fmt.Errorf("failed to load existing config at %s", configPath)
	}

	if strings.TrimSpace(fileCfg.Server.Token) == "" {
		token, err := config.GenerateAuthToken()
		if err == nil {
			fileCfg.Server.Token = token
			_ = config.SaveFileConfig(fileCfg, configPath)
			return configPath, fileCfg, ConfigRecovered, nil
		}
	}

	return configPath, fileCfg, ConfigVerified, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
