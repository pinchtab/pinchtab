package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvOr(t *testing.T) {
	key := "PINCHTAB_TEST_ENV"
	fallback := "default"

	_ = os.Unsetenv(key)
	if got := envOr(key, fallback); got != fallback {
		t.Errorf("envOr() = %v, want %v", got, fallback)
	}

	val := "set"
	_ = os.Setenv(key, val)
	defer func() { _ = os.Unsetenv(key) }()
	if got := envOr(key, fallback); got != val {
		t.Errorf("envOr() = %v, want %v", got, val)
	}
}

func TestEnvIntOr(t *testing.T) {
	key := "PINCHTAB_TEST_INT"
	fallback := 42

	_ = os.Unsetenv(key)
	if got := envIntOr(key, fallback); got != fallback {
		t.Errorf("envIntOr() = %v, want %v", got, fallback)
	}

	_ = os.Setenv(key, "100")
	if got := envIntOr(key, fallback); got != 100 {
		t.Errorf("envIntOr() = %v, want %v", got, 100)
	}

	_ = os.Setenv(key, "invalid")
	if got := envIntOr(key, fallback); got != fallback {
		t.Errorf("envIntOr() = %v, want %v", got, fallback)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnvVars(t)
	// Point to non-existent config to test pure defaults
	_ = os.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "nonexistent.json"))
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	cfg := Load()
	if cfg.Port != "9867" {
		t.Errorf("default Port = %v, want 9867", cfg.Port)
	}
	if cfg.Bind != "127.0.0.1" {
		t.Errorf("default Bind = %v, want 127.0.0.1", cfg.Bind)
	}
	if cfg.AllowEvaluate {
		t.Errorf("default AllowEvaluate = %v, want false", cfg.AllowEvaluate)
	}
	if cfg.Strategy != "always-on" {
		t.Errorf("default Strategy = %v, want always-on", cfg.Strategy)
	}
	if cfg.AllocationPolicy != "fcfs" {
		t.Errorf("default AllocationPolicy = %v, want fcfs", cfg.AllocationPolicy)
	}
	if cfg.TabEvictionPolicy != "close_lru" {
		t.Errorf("default TabEvictionPolicy = %v, want close_lru", cfg.TabEvictionPolicy)
	}
	if !cfg.AttachEnabled {
		t.Errorf("default AttachEnabled = %v, want true", cfg.AttachEnabled)
	}
	if len(cfg.AttachAllowSchemes) != 2 || cfg.AttachAllowSchemes[0] != "ws" || cfg.AttachAllowSchemes[1] != "wss" {
		t.Errorf("default AttachAllowSchemes = %v, want [ws wss]", cfg.AttachAllowSchemes)
	}
	if !cfg.IDPI.Enabled {
		t.Errorf("default IDPI.Enabled = %v, want true", cfg.IDPI.Enabled)
	}
	if len(cfg.IDPI.AllowedDomains) != 3 || cfg.IDPI.AllowedDomains[0] != "127.0.0.1" {
		t.Errorf("default IDPI.AllowedDomains = %v, want local-only allowlist", cfg.IDPI.AllowedDomains)
	}
	if !cfg.IDPI.StrictMode {
		t.Errorf("default IDPI.StrictMode = %v, want true", cfg.IDPI.StrictMode)
	}
	if !cfg.IDPI.ScanContent {
		t.Errorf("default IDPI.ScanContent = %v, want true", cfg.IDPI.ScanContent)
	}
	if !cfg.IDPI.WrapContent {
		t.Errorf("default IDPI.WrapContent = %v, want true", cfg.IDPI.WrapContent)
	}
}

func TestLoadConfigTokenEnvOverride(t *testing.T) {
	clearConfigEnvVars(t)
	// Point to non-existent config to isolate env var testing
	_ = os.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "nonexistent.json"))
	_ = os.Setenv("PINCHTAB_TOKEN", "secret")
	defer func() {
		_ = os.Unsetenv("PINCHTAB_CONFIG")
		_ = os.Unsetenv("PINCHTAB_TOKEN")
	}()

	cfg := Load()
	// Port and Bind use defaults (no env var override anymore)
	if cfg.Port != "9867" {
		t.Errorf("default Port = %v, want 9867", cfg.Port)
	}
	if cfg.Bind != "127.0.0.1" {
		t.Errorf("default Bind = %v, want 127.0.0.1", cfg.Bind)
	}
	// Token still supports env var override
	if cfg.Token != "secret" {
		t.Errorf("env Token = %v, want secret", cfg.Token)
	}
}

func TestConfigFilePortOverridesDefault(t *testing.T) {
	clearConfigEnvVars(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() {
		_ = os.Unsetenv("PINCHTAB_CONFIG")
	}()

	if err := os.WriteFile(configPath, []byte(`{"server":{"port":"8888"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Port != "8888" {
		t.Errorf("config file Port = %v, want 8888", cfg.Port)
	}
}

func TestConfigFileWithNestedValues(t *testing.T) {
	clearConfigEnvVars(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() {
		_ = os.Unsetenv("PINCHTAB_CONFIG")
	}()

	// Config file says port 8888 and strategy explicit
	nestedConfig := `{
		"server": {
			"port": "8888"
		},
		"multiInstance": {
			"strategy": "explicit"
		}
	}`
	if err := os.WriteFile(configPath, []byte(nestedConfig), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()

	// Config file values should be used
	if cfg.Port != "8888" {
		t.Errorf("config file Port = %v, want 8888", cfg.Port)
	}
	if cfg.Strategy != "explicit" {
		t.Errorf("config file Strategy = %v, want explicit", cfg.Strategy)
	}
}

func TestLoadConfigEngineFromFile(t *testing.T) {
	clearConfigEnvVars(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	if err := os.WriteFile(configPath, []byte(`{"server":{"engine":"lite"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Engine != "lite" {
		t.Fatalf("engine = %q, want lite", cfg.Engine)
	}
}

func TestApplyFileConfigToRuntimeResetsSecurityFlagsToSafeDefaults(t *testing.T) {
	cfg := &RuntimeConfig{
		AllowEvaluate:   true,
		AllowMacro:      true,
		AllowScreencast: true,
		AllowDownload:   true,
		AllowUpload:     true,
		IDPI: IDPIConfig{
			Enabled: false,
		},
	}

	fc := DefaultFileConfig()
	ApplyFileConfigToRuntime(cfg, &fc)

	if cfg.AllowEvaluate {
		t.Errorf("ApplyFileConfigToRuntime AllowEvaluate = %v, want false", cfg.AllowEvaluate)
	}
	if cfg.AllowMacro {
		t.Errorf("ApplyFileConfigToRuntime AllowMacro = %v, want false", cfg.AllowMacro)
	}
	if cfg.AllowScreencast {
		t.Errorf("ApplyFileConfigToRuntime AllowScreencast = %v, want false", cfg.AllowScreencast)
	}
	if cfg.AllowDownload {
		t.Errorf("ApplyFileConfigToRuntime AllowDownload = %v, want false", cfg.AllowDownload)
	}
	if cfg.AllowUpload {
		t.Errorf("ApplyFileConfigToRuntime AllowUpload = %v, want false", cfg.AllowUpload)
	}
	if !cfg.IDPI.Enabled {
		t.Errorf("ApplyFileConfigToRuntime IDPI.Enabled = %v, want true", cfg.IDPI.Enabled)
	}
	if len(cfg.IDPI.AllowedDomains) != 3 || cfg.IDPI.AllowedDomains[0] != "127.0.0.1" {
		t.Errorf("ApplyFileConfigToRuntime IDPI.AllowedDomains = %v, want local-only allowlist", cfg.IDPI.AllowedDomains)
	}
	if !cfg.IDPI.StrictMode || !cfg.IDPI.ScanContent || !cfg.IDPI.WrapContent {
		t.Errorf("ApplyFileConfigToRuntime IDPI = %+v, want strict+scan+wrap enabled", cfg.IDPI)
	}
}

func TestApplyFileConfigToRuntime_ClampsNetworkBufferSize(t *testing.T) {
	cfg := &RuntimeConfig{}
	oversized := MaxNetworkBufferSize + 1
	fc := &FileConfig{
		Server: ServerConfig{NetworkBufferSize: &oversized},
	}

	ApplyFileConfigToRuntime(cfg, fc)

	if cfg.NetworkBufferSize != MaxNetworkBufferSize {
		t.Errorf("ApplyFileConfigToRuntime NetworkBufferSize = %d, want %d", cfg.NetworkBufferSize, MaxNetworkBufferSize)
	}
}

// clearConfigEnvVars unsets all config-related env vars for clean tests.
func clearConfigEnvVars(t *testing.T) {
	t.Helper()
	envVars := []string{
		"PINCHTAB_TOKEN", "PINCHTAB_CONFIG",
	}
	for _, v := range envVars {
		_ = os.Unsetenv(v)
	}
}
