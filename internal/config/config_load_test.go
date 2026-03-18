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
	if len(cfg.DownloadAllowedDomains) != 0 {
		t.Errorf("default DownloadAllowedDomains = %v, want empty list", cfg.DownloadAllowedDomains)
	}
	if cfg.DownloadMaxBytes != DefaultDownloadMaxBytes {
		t.Errorf("default DownloadMaxBytes = %d, want %d", cfg.DownloadMaxBytes, DefaultDownloadMaxBytes)
	}
	if cfg.UploadMaxRequestBytes != DefaultUploadMaxRequestBytes {
		t.Errorf("default UploadMaxRequestBytes = %d, want %d", cfg.UploadMaxRequestBytes, DefaultUploadMaxRequestBytes)
	}
	if cfg.UploadMaxFiles != DefaultUploadMaxFiles {
		t.Errorf("default UploadMaxFiles = %d, want %d", cfg.UploadMaxFiles, DefaultUploadMaxFiles)
	}
	if cfg.UploadMaxFileBytes != DefaultUploadMaxFileBytes {
		t.Errorf("default UploadMaxFileBytes = %d, want %d", cfg.UploadMaxFileBytes, DefaultUploadMaxFileBytes)
	}
	if cfg.UploadMaxTotalBytes != DefaultUploadMaxTotalBytes {
		t.Errorf("default UploadMaxTotalBytes = %d, want %d", cfg.UploadMaxTotalBytes, DefaultUploadMaxTotalBytes)
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
	if cfg.AttachEnabled {
		t.Errorf("default AttachEnabled = %v, want false", cfg.AttachEnabled)
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
	if len(cfg.DownloadAllowedDomains) != 0 {
		t.Errorf("ApplyFileConfigToRuntime DownloadAllowedDomains = %v, want empty list", cfg.DownloadAllowedDomains)
	}
	if cfg.DownloadMaxBytes != DefaultDownloadMaxBytes {
		t.Errorf("ApplyFileConfigToRuntime DownloadMaxBytes = %d, want %d", cfg.DownloadMaxBytes, DefaultDownloadMaxBytes)
	}
	if cfg.UploadMaxRequestBytes != DefaultUploadMaxRequestBytes {
		t.Errorf("ApplyFileConfigToRuntime UploadMaxRequestBytes = %d, want %d", cfg.UploadMaxRequestBytes, DefaultUploadMaxRequestBytes)
	}
	if cfg.UploadMaxFiles != DefaultUploadMaxFiles {
		t.Errorf("ApplyFileConfigToRuntime UploadMaxFiles = %d, want %d", cfg.UploadMaxFiles, DefaultUploadMaxFiles)
	}
	if cfg.UploadMaxFileBytes != DefaultUploadMaxFileBytes {
		t.Errorf("ApplyFileConfigToRuntime UploadMaxFileBytes = %d, want %d", cfg.UploadMaxFileBytes, DefaultUploadMaxFileBytes)
	}
	if cfg.UploadMaxTotalBytes != DefaultUploadMaxTotalBytes {
		t.Errorf("ApplyFileConfigToRuntime UploadMaxTotalBytes = %d, want %d", cfg.UploadMaxTotalBytes, DefaultUploadMaxTotalBytes)
	}
	if cfg.AttachEnabled {
		t.Errorf("ApplyFileConfigToRuntime AttachEnabled = %v, want false", cfg.AttachEnabled)
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

func TestApplyFileConfigToRuntimeClearsTokenWhenFileTokenRemoved(t *testing.T) {
	clearConfigEnvVars(t)

	cfg := &RuntimeConfig{Token: "secret-token"}
	fc := DefaultFileConfig()
	fc.Server.Token = ""

	ApplyFileConfigToRuntime(cfg, &fc)

	if cfg.Token != "" {
		t.Fatalf("ApplyFileConfigToRuntime Token = %q, want empty string", cfg.Token)
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

func TestApplyFileConfigToRuntime_CopiesDownloadAllowedDomains(t *testing.T) {
	cfg := &RuntimeConfig{}
	fc := &FileConfig{
		Security: SecurityConfig{
			DownloadAllowedDomains: []string{"pinchtab.com", "*.pinchtab.com"},
		},
	}

	ApplyFileConfigToRuntime(cfg, fc)
	fc.Security.DownloadAllowedDomains[0] = "mutated.example.com"

	if len(cfg.DownloadAllowedDomains) != 2 {
		t.Fatalf("ApplyFileConfigToRuntime DownloadAllowedDomains = %v, want 2 entries", cfg.DownloadAllowedDomains)
	}
	if cfg.DownloadAllowedDomains[0] != "pinchtab.com" {
		t.Fatalf("ApplyFileConfigToRuntime copied list = %v, want original values", cfg.DownloadAllowedDomains)
	}
}

func TestRuntimeConfig_EffectiveTransferLimitsFallbackAndClamp(t *testing.T) {
	cfg := &RuntimeConfig{}
	if cfg.EffectiveDownloadMaxBytes() != DefaultDownloadMaxBytes {
		t.Fatalf("EffectiveDownloadMaxBytes() = %d, want %d", cfg.EffectiveDownloadMaxBytes(), DefaultDownloadMaxBytes)
	}
	if cfg.EffectiveUploadMaxRequestBytes() != DefaultUploadMaxRequestBytes {
		t.Fatalf("EffectiveUploadMaxRequestBytes() = %d, want %d", cfg.EffectiveUploadMaxRequestBytes(), DefaultUploadMaxRequestBytes)
	}
	if cfg.EffectiveUploadMaxFiles() != DefaultUploadMaxFiles {
		t.Fatalf("EffectiveUploadMaxFiles() = %d, want %d", cfg.EffectiveUploadMaxFiles(), DefaultUploadMaxFiles)
	}
	if cfg.EffectiveUploadMaxFileBytes() != DefaultUploadMaxFileBytes {
		t.Fatalf("EffectiveUploadMaxFileBytes() = %d, want %d", cfg.EffectiveUploadMaxFileBytes(), DefaultUploadMaxFileBytes)
	}
	if cfg.EffectiveUploadMaxTotalBytes() != DefaultUploadMaxTotalBytes {
		t.Fatalf("EffectiveUploadMaxTotalBytes() = %d, want %d", cfg.EffectiveUploadMaxTotalBytes(), DefaultUploadMaxTotalBytes)
	}

	cfg.DownloadMaxBytes = MaxDownloadMaxBytes + 1
	cfg.UploadMaxRequestBytes = MaxUploadMaxRequestBytes + 1
	cfg.UploadMaxFiles = MaxUploadMaxFiles + 1
	cfg.UploadMaxFileBytes = MaxUploadMaxFileBytes + 1
	cfg.UploadMaxTotalBytes = MaxUploadMaxTotalBytes + 1

	if cfg.EffectiveDownloadMaxBytes() != MaxDownloadMaxBytes {
		t.Fatalf("EffectiveDownloadMaxBytes clamp = %d, want %d", cfg.EffectiveDownloadMaxBytes(), MaxDownloadMaxBytes)
	}
	if cfg.EffectiveUploadMaxRequestBytes() != MaxUploadMaxRequestBytes {
		t.Fatalf("EffectiveUploadMaxRequestBytes clamp = %d, want %d", cfg.EffectiveUploadMaxRequestBytes(), MaxUploadMaxRequestBytes)
	}
	if cfg.EffectiveUploadMaxFiles() != MaxUploadMaxFiles {
		t.Fatalf("EffectiveUploadMaxFiles clamp = %d, want %d", cfg.EffectiveUploadMaxFiles(), MaxUploadMaxFiles)
	}
	if cfg.EffectiveUploadMaxFileBytes() != MaxUploadMaxFileBytes {
		t.Fatalf("EffectiveUploadMaxFileBytes clamp = %d, want %d", cfg.EffectiveUploadMaxFileBytes(), MaxUploadMaxFileBytes)
	}
	if cfg.EffectiveUploadMaxTotalBytes() != MaxUploadMaxTotalBytes {
		t.Fatalf("EffectiveUploadMaxTotalBytes clamp = %d, want %d", cfg.EffectiveUploadMaxTotalBytes(), MaxUploadMaxTotalBytes)
	}
}

func TestApplyFileConfigToRuntime_ClampsTransferLimits(t *testing.T) {
	cfg := &RuntimeConfig{}
	downloadTooLarge := MaxDownloadMaxBytes + 1
	requestTooLarge := MaxUploadMaxRequestBytes + 1
	filesTooLarge := MaxUploadMaxFiles + 1
	fileTooLarge := MaxUploadMaxFileBytes + 1
	totalTooLarge := MaxUploadMaxTotalBytes + 1
	fc := &FileConfig{
		Security: SecurityConfig{
			DownloadMaxBytes:      &downloadTooLarge,
			UploadMaxRequestBytes: &requestTooLarge,
			UploadMaxFiles:        &filesTooLarge,
			UploadMaxFileBytes:    &fileTooLarge,
			UploadMaxTotalBytes:   &totalTooLarge,
		},
	}

	ApplyFileConfigToRuntime(cfg, fc)

	if cfg.DownloadMaxBytes != MaxDownloadMaxBytes {
		t.Fatalf("DownloadMaxBytes = %d, want %d", cfg.DownloadMaxBytes, MaxDownloadMaxBytes)
	}
	if cfg.UploadMaxRequestBytes != MaxUploadMaxRequestBytes {
		t.Fatalf("UploadMaxRequestBytes = %d, want %d", cfg.UploadMaxRequestBytes, MaxUploadMaxRequestBytes)
	}
	if cfg.UploadMaxFiles != MaxUploadMaxFiles {
		t.Fatalf("UploadMaxFiles = %d, want %d", cfg.UploadMaxFiles, MaxUploadMaxFiles)
	}
	if cfg.UploadMaxFileBytes != MaxUploadMaxFileBytes {
		t.Fatalf("UploadMaxFileBytes = %d, want %d", cfg.UploadMaxFileBytes, MaxUploadMaxFileBytes)
	}
	if cfg.UploadMaxTotalBytes != MaxUploadMaxTotalBytes {
		t.Fatalf("UploadMaxTotalBytes = %d, want %d", cfg.UploadMaxTotalBytes, MaxUploadMaxTotalBytes)
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
