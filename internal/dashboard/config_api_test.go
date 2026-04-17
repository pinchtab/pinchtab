package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

type stubAgentCounter struct {
	count int
}

func (s stubAgentCounter) AgentCount() int { return s.count }

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
	api := NewConfigAPI(runtime, nil, nil, nil, nil, "test", time.Now())

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
	api := NewConfigAPI(config.Load(), nil, nil, nil, nil, "test", time.Now())
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
	stateKey := "state-secret"
	fc.Security.StateEncryptionKey = &stateKey
	fc.AutoSolver.External.CapsolverKey = "capsolver-secret"
	fc.AutoSolver.External.TwoCaptchaKey = "twocaptcha-secret"

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
	if env.Config.Security.StateEncryptionKey != nil {
		t.Fatalf("config stateEncryptionKey = %v, want nil", env.Config.Security.StateEncryptionKey)
	}
	if env.Config.AutoSolver.External.CapsolverKey != "" {
		t.Fatalf("config capsolverKey = %q, want redacted empty string", env.Config.AutoSolver.External.CapsolverKey)
	}
	if env.Config.AutoSolver.External.TwoCaptchaKey != "" {
		t.Fatalf("config twoCaptchaKey = %q, want redacted empty string", env.Config.AutoSolver.External.TwoCaptchaKey)
	}
	if !env.TokenConfigured {
		t.Fatal("tokenConfigured = false, want true")
	}
}

func TestHandlePutConfigPreservesExistingToken(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	stateKey := "state-secret"
	fc.Security.StateEncryptionKey = &stateKey
	fc.AutoSolver.External.CapsolverKey = "capsolver-secret"
	fc.AutoSolver.External.TwoCaptchaKey = "twocaptcha-secret"

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
	if saved.Security.StateEncryptionKey == nil || *saved.Security.StateEncryptionKey != stateKey {
		t.Fatalf("saved stateEncryptionKey = %v, want existing key preserved", saved.Security.StateEncryptionKey)
	}
	if saved.AutoSolver.External.CapsolverKey != "capsolver-secret" {
		t.Fatalf("saved capsolverKey = %q, want existing key preserved", saved.AutoSolver.External.CapsolverKey)
	}
	if saved.AutoSolver.External.TwoCaptchaKey != "twocaptcha-secret" {
		t.Fatalf("saved twoCaptchaKey = %q, want existing key preserved", saved.AutoSolver.External.TwoCaptchaKey)
	}
	if saved.Server.Port != "9999" {
		t.Fatalf("saved port = %q, want %q", saved.Server.Port, "9999")
	}
}

func TestHandlePutConfigPreservesWriteOnlySecretsFromRedactedGetPayload(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	stateKey := "state-secret"
	fc.Security.StateEncryptionKey = &stateKey
	fc.AutoSolver.External.CapsolverKey = "capsolver-secret"
	fc.AutoSolver.External.TwoCaptchaKey = "twocaptcha-secret"

	api := newConfigAPITestAPI(t, fc)

	getReq := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	getRes := httptest.NewRecorder()
	api.HandleGetConfig(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("HandleGetConfig() status = %d, want %d", getRes.Code, http.StatusOK)
	}

	env := decodeConfigEnvelope(t, getRes)
	env.Config.Server.Port = "9898"

	body, err := json.Marshal(env.Config)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	putRes := httptest.NewRecorder()
	api.HandlePutConfig(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d", putRes.Code, http.StatusOK)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Server.Token != "secret-token" {
		t.Fatalf("saved token = %q, want existing token preserved", saved.Server.Token)
	}
	if saved.Security.StateEncryptionKey == nil || *saved.Security.StateEncryptionKey != stateKey {
		t.Fatalf("saved stateEncryptionKey = %v, want existing key preserved", saved.Security.StateEncryptionKey)
	}
	if saved.AutoSolver.External.CapsolverKey != "capsolver-secret" {
		t.Fatalf("saved capsolverKey = %q, want existing key preserved", saved.AutoSolver.External.CapsolverKey)
	}
	if saved.AutoSolver.External.TwoCaptchaKey != "twocaptcha-secret" {
		t.Fatalf("saved twoCaptchaKey = %q, want existing key preserved", saved.AutoSolver.External.TwoCaptchaKey)
	}
	if saved.Server.Port != "9898" {
		t.Fatalf("saved port = %q, want %q", saved.Server.Port, "9898")
	}
}

func TestHandlePutConfigPreservesUnspecifiedAttachSettings(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	attachEnabled := true
	fc.Security.Attach.Enabled = &attachEnabled
	fc.Security.Attach.AllowHosts = []string{"127.0.0.1", "pinchtab-bridge"}
	fc.Security.Attach.AllowSchemes = []string{"http", "https"}

	api := newConfigAPITestAPI(t, fc)

	body := []byte(`{"server":{"trustProxyHeaders":true}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Security.Attach.Enabled == nil || !*saved.Security.Attach.Enabled {
		t.Fatalf("saved attach.enabled = %v, want true", saved.Security.Attach.Enabled)
	}
	if len(saved.Security.Attach.AllowHosts) != 2 || saved.Security.Attach.AllowHosts[1] != "pinchtab-bridge" {
		t.Fatalf("saved attach.allowHosts = %v, want preserved hosts", saved.Security.Attach.AllowHosts)
	}
	if len(saved.Security.Attach.AllowSchemes) != 2 || saved.Security.Attach.AllowSchemes[0] != "http" {
		t.Fatalf("saved attach.allowSchemes = %v, want preserved schemes", saved.Security.Attach.AllowSchemes)
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

func TestHandleHealthIncludesAgentCount(t *testing.T) {
	fc := config.DefaultFileConfig()
	api := newConfigAPITestAPI(t, fc)
	api.agents = stubAgentCounter{count: 3}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	api.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var health healthEnvelope
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if health.Agents != 3 {
		t.Fatalf("health agents = %d, want 3", health.Agents)
	}
}

func newConfigAPITestAPI(t *testing.T, fc config.FileConfig) *ConfigAPI {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	return NewConfigAPI(config.Load(), nil, nil, nil, nil, "test", time.Now())
}

func decodeConfigEnvelope(t *testing.T, w *httptest.ResponseRecorder) configEnvelope {
	t.Helper()

	var env configEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return env
}

// TestRedactTokenCoversAllSensitiveFields uses reflection to find fields that look
// like secrets (containing "key", "token", "secret", "password", "credential" in their
// name) and verifies they are all redacted. This test will fail if a new sensitive
// field is added to FileConfig without updating redactToken().
func TestRedactTokenCoversAllSensitiveFields(t *testing.T) {
	// Populate all known sensitive fields with non-zero values
	fc := config.DefaultFileConfig()
	fc.Server.Token = "test-token"
	encKey := "test-encryption-key"
	fc.Security.StateEncryptionKey = &encKey
	fc.AutoSolver.External.CapsolverKey = "test-capsolver-key"
	fc.AutoSolver.External.TwoCaptchaKey = "test-twocaptcha-key"

	redacted := redactToken(fc)

	// Use reflection to find any sensitive fields that weren't redacted
	var unredacted []string
	findSensitiveFields(reflect.ValueOf(redacted), "", &unredacted)

	if len(unredacted) > 0 {
		t.Fatalf("redactToken() did not redact sensitive fields: %v\n"+
			"Add these to redactToken() in config_api.go", unredacted)
	}
}

// findSensitiveFields recursively scans a struct for fields with names suggesting
// they contain secrets, and reports any that have non-zero values.
func findSensitiveFields(v reflect.Value, path string, unredacted *[]string) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	sensitivePatterns := []string{"key", "token", "secret", "password", "credential"}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldPath := field.Name
		if path != "" {
			fieldPath = path + "." + field.Name
		}

		fieldVal := v.Field(i)

		// Check if field name suggests it's sensitive
		nameLower := strings.ToLower(field.Name)
		isSensitive := false
		for _, pattern := range sensitivePatterns {
			if strings.Contains(nameLower, pattern) {
				isSensitive = true
				break
			}
		}

		if isSensitive && !isZeroValue(fieldVal) {
			*unredacted = append(*unredacted, fieldPath)
		}

		// Recurse into nested structs
		if fieldVal.Kind() == reflect.Struct || (fieldVal.Kind() == reflect.Ptr && fieldVal.Elem().Kind() == reflect.Struct) {
			findSensitiveFields(fieldVal, fieldPath, unredacted)
		}
	}
}

func isZeroValue(v reflect.Value) bool {
	if v.Kind() == reflect.Ptr {
		return v.IsNil()
	}
	return v.IsZero()
}
