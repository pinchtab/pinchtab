package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestHandleGetConfigRedactsToken(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"

	api := newConfigAPITestAPI(t, fc)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	api.HandleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleGetConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	env := decodeConfigEnvelope(t, w)
	if env.Config.Server.Token != "" {
		t.Fatalf("config token = %q, want redacted empty string", env.Config.Server.Token)
	}
	if !env.TokenConfigured {
		t.Fatal("tokenConfigured = false, want true")
	}
}

func TestHandlePutConfigPreservesExistingToken(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"

	api := newConfigAPITestAPI(t, fc)

	payload := config.DefaultFileConfig()
	payload.Server.Port = "9999"

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	env := decodeConfigEnvelope(t, w)
	if env.Config.Server.Token != "" {
		t.Fatalf("response token = %q, want redacted empty string", env.Config.Server.Token)
	}
	if !env.TokenConfigured {
		t.Fatal("tokenConfigured = false, want true")
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Server.Token != "secret-token" {
		t.Fatalf("saved token = %q, want existing token preserved", saved.Server.Token)
	}
	if saved.Server.Port != "9999" {
		t.Fatalf("saved port = %q, want %q", saved.Server.Port, "9999")
	}
}

func TestHandlePutConfigRejectsWriteOnlyTokenField(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"

	api := newConfigAPITestAPI(t, fc)

	payload := config.DefaultFileConfig()
	payload.Server.Token = "replacement-token"

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("HandlePutConfig() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "token_write_only") {
		t.Fatalf("response = %q, want token_write_only error", w.Body.String())
	}
}

func newConfigAPITestAPI(t *testing.T, fc config.FileConfig) *ConfigAPI {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	return NewConfigAPI(config.Load(), nil, nil, nil, "test", time.Now())
}

func decodeConfigEnvelope(t *testing.T, w *httptest.ResponseRecorder) configEnvelope {
	t.Helper()

	var env configEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return env
}
