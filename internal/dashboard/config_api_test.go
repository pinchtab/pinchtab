package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestNewConfigAPISnapshotsBootConfigFromFile(t *testing.T) {
	defaults := config.DefaultFileConfig()
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fileConfig := map[string]any{
		"configVersion": defaults.ConfigVersion,
		"server": map[string]any{
			"port":     defaults.Server.Port,
			"bind":     defaults.Server.Bind,
			"stateDir": defaults.Server.StateDir,
		},
		"profiles": map[string]any{
			"baseDir":        defaults.Profiles.BaseDir,
			"defaultProfile": defaults.Profiles.DefaultProfile,
		},
		"multiInstance": map[string]any{
			"strategy": defaults.MultiInstance.Strategy,
			"restart": map[string]any{
				"maxRestarts":    nil,
				"initBackoffSec": nil,
				"maxBackoffSec":  nil,
				"stableAfterSec": nil,
			},
		},
	}

	data, err := json.Marshal(fileConfig)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := config.Load()
	api := NewConfigAPI(runtime, nil, nil, nil, "test", time.Now())

	if api.boot.MultiInstance.Restart.MaxRestarts != nil {
		t.Fatalf("boot restart maxRestarts = %v, want nil from file snapshot", *api.boot.MultiInstance.Restart.MaxRestarts)
	}

	_, path, restartReasons, err := api.currentConfig()
	if err != nil {
		t.Fatalf("currentConfig() error = %v", err)
	}
	if path != configPath {
		t.Fatalf("currentConfig() path = %q, want %q", path, configPath)
	}
	if len(restartReasons) != 0 {
		t.Fatalf("currentConfig() restartReasons = %v, want none", restartReasons)
	}
}

func TestRestartReasonsIncludeStealthLevel(t *testing.T) {
	cfg := config.DefaultFileConfig()
	api := NewConfigAPI(config.Load(), nil, nil, nil, "test", time.Now())
	api.boot = cfg

	next := cfg
	next.InstanceDefaults.StealthLevel = "full"

	reasons := api.restartReasonsFor(next)
	if !slices.Contains(reasons, "Stealth level") {
		t.Fatalf("restartReasonsFor() = %v, want Stealth level", reasons)
	}
}
