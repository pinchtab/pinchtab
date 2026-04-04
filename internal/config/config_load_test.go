package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if !cfg.EnableActionGuards {
		t.Errorf("default EnableActionGuards = %v, want true", cfg.EnableActionGuards)
	}
	if cfg.TrustProxyHeaders {
		t.Errorf("default TrustProxyHeaders = %v, want false", cfg.TrustProxyHeaders)
	}
	if cfg.CookieSecure != nil {
		t.Errorf("default CookieSecure = %v, want nil for auto-detect", *cfg.CookieSecure)
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
	if !cfg.Observability.Activity.Enabled {
		t.Errorf("default Observability.Activity.Enabled = %v, want true", cfg.Observability.Activity.Enabled)
	}
	if cfg.Observability.Activity.RetentionDays != 1 {
		t.Errorf("default Observability.Activity.RetentionDays = %d, want 1", cfg.Observability.Activity.RetentionDays)
	}
	if !cfg.Sessions.Dashboard.Persist {
		t.Errorf("default Sessions.Dashboard.Persist = %v, want true", cfg.Sessions.Dashboard.Persist)
	}
	if cfg.Sessions.Dashboard.IdleTimeout != 7*24*time.Hour {
		t.Errorf("default Sessions.Dashboard.IdleTimeout = %v, want %v", cfg.Sessions.Dashboard.IdleTimeout, 7*24*time.Hour)
	}
	if cfg.Sessions.Dashboard.MaxLifetime != 7*24*time.Hour {
		t.Errorf("default Sessions.Dashboard.MaxLifetime = %v, want %v", cfg.Sessions.Dashboard.MaxLifetime, 7*24*time.Hour)
	}
	if cfg.Sessions.Dashboard.RequireElevation {
		t.Errorf("default Sessions.Dashboard.RequireElevation = %v, want false", cfg.Sessions.Dashboard.RequireElevation)
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
		"instanceDefaults": {
			"maxParallelTabs": 4
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
	if cfg.MaxParallelTabs != 4 {
		t.Errorf("config file MaxParallelTabs = %v, want 4", cfg.MaxParallelTabs)
	}
	if cfg.Strategy != "explicit" {
		t.Errorf("config file Strategy = %v, want explicit", cfg.Strategy)
	}
}

func TestLoadConfigActivityStateDirIgnoresConfigOverride(t *testing.T) {
	clearConfigEnvVars(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	sharedActivityDir := filepath.Join(tmpDir, "shared-activity")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	if err := os.WriteFile(configPath, []byte(`{
		"server": {
			"stateDir": "/tmp/profile-state"
		},
		"observability": {
			"activity": {
				"stateDir": "`+sharedActivityDir+`"
			}
		}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Observability.Activity.StateDir != "" {
		t.Fatalf("Observability.Activity.StateDir = %q, want empty", cfg.Observability.Activity.StateDir)
	}
	if cfg.ActivityStateDir() != "/tmp/profile-state" {
		t.Fatalf("ActivityStateDir() = %q, want %q", cfg.ActivityStateDir(), "/tmp/profile-state")
	}
}

func TestRuntimeConfigActivityStateDirFallsBackToStateDir(t *testing.T) {
	cfg := &RuntimeConfig{StateDir: "/tmp/pinchtab-state"}

	if got := cfg.ActivityStateDir(); got != "/tmp/pinchtab-state" {
		t.Fatalf("ActivityStateDir() = %q, want %q", got, "/tmp/pinchtab-state")
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
	if !cfg.EnableActionGuards {
		t.Errorf("ApplyFileConfigToRuntime EnableActionGuards = %v, want true", cfg.EnableActionGuards)
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

func TestLoadPreservesIDPIShieldThreshold(t *testing.T) {
	clearConfigEnvVars(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	if err := os.WriteFile(configPath, []byte(`{
		"security": {
			"idpi": {
				"enabled": true,
				"strictMode": true,
				"scanContent": true,
				"wrapContent": true,
				"allowedDomains": ["fixtures"],
				"shieldThreshold": 30
			}
		}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.IDPI.ShieldThreshold != 30 {
		t.Fatalf("IDPI.ShieldThreshold = %d, want 30", cfg.IDPI.ShieldThreshold)
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

func TestApplyFileConfigToRuntime_CopiesAttachConfig(t *testing.T) {
	cfg := &RuntimeConfig{}
	enabled := true
	fc := &FileConfig{
		Security: SecurityConfig{
			Attach: AttachConfig{
				Enabled:      &enabled,
				AllowHosts:   []string{"127.0.0.1", "pinchtab-bridge"},
				AllowSchemes: []string{"http", "https"},
			},
		},
	}

	ApplyFileConfigToRuntime(cfg, fc)
	fc.Security.Attach.AllowHosts[0] = "mutated.example.com"
	fc.Security.Attach.AllowSchemes[0] = "ws"

	if !cfg.AttachEnabled {
		t.Fatalf("ApplyFileConfigToRuntime AttachEnabled = %v, want true", cfg.AttachEnabled)
	}
	if len(cfg.AttachAllowHosts) != 2 || cfg.AttachAllowHosts[1] != "pinchtab-bridge" {
		t.Fatalf("ApplyFileConfigToRuntime AttachAllowHosts = %v, want copied hosts", cfg.AttachAllowHosts)
	}
	if len(cfg.AttachAllowSchemes) != 2 || cfg.AttachAllowSchemes[0] != "http" {
		t.Fatalf("ApplyFileConfigToRuntime AttachAllowSchemes = %v, want copied schemes", cfg.AttachAllowSchemes)
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

func TestApplyFileConfigToRuntime_TrustProxyHeaders(t *testing.T) {
	cfg := &RuntimeConfig{}
	if cfg.TrustProxyHeaders {
		t.Fatal("expected default TrustProxyHeaders to be false")
	}

	enabled := true
	fc := &FileConfig{Server: ServerConfig{TrustProxyHeaders: &enabled}}
	applyFileConfig(cfg, fc)
	if !cfg.TrustProxyHeaders {
		t.Fatal("expected TrustProxyHeaders to be true after apply")
	}

	disabled := false
	fc2 := &FileConfig{Server: ServerConfig{TrustProxyHeaders: &disabled}}
	applyFileConfig(cfg, fc2)
	if cfg.TrustProxyHeaders {
		t.Fatal("expected TrustProxyHeaders to be false after apply with false")
	}
}

func TestApplyFileConfigToRuntime_CookieSecure(t *testing.T) {
	cfg := &RuntimeConfig{}
	if cfg.CookieSecure != nil {
		t.Fatal("expected default CookieSecure to be nil")
	}

	enabled := true
	fc := &FileConfig{Server: ServerConfig{CookieSecure: &enabled}}
	applyFileConfig(cfg, fc)
	if cfg.CookieSecure == nil || !*cfg.CookieSecure {
		t.Fatal("expected CookieSecure to be true after apply")
	}

	disabled := false
	fc2 := &FileConfig{Server: ServerConfig{CookieSecure: &disabled}}
	applyFileConfig(cfg, fc2)
	if cfg.CookieSecure == nil || *cfg.CookieSecure {
		t.Fatal("expected CookieSecure to be false after apply with false")
	}

	fc3 := &FileConfig{}
	applyFileConfig(cfg, fc3)
	if cfg.CookieSecure != nil {
		t.Fatal("expected CookieSecure to reset to nil when omitted")
	}
}

func TestApplyFileConfigToRuntime_SanitizesChromeExtraFlags(t *testing.T) {
	cfg := &RuntimeConfig{}
	fc := &FileConfig{
		Browser: BrowserConfig{
			ChromeExtraFlags: "--disable-gpu --user-agent=Bad/1.0 --disable-web-security --ash-no-nudges",
		},
	}

	ApplyFileConfigToRuntime(cfg, fc)

	if cfg.ChromeExtraFlags != "--disable-gpu --ash-no-nudges" {
		t.Fatalf("ChromeExtraFlags = %q, want %q", cfg.ChromeExtraFlags, "--disable-gpu --ash-no-nudges")
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
