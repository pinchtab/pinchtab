package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDefaultFileConfig(t *testing.T) {
	setCloakBrowserDiscovery(t, "")
	fc := DefaultFileConfig()
	if fc.Schema != CurrentConfigSchemaURL() {
		t.Errorf("DefaultFileConfig.Schema = %q, want %q", fc.Schema, CurrentConfigSchemaURL())
	}
	if fc.Server.Port != "9867" {
		t.Errorf("DefaultFileConfig.Server.Port = %v, want 9867", fc.Server.Port)
	}
	if fc.Server.Bind != "127.0.0.1" {
		t.Errorf("DefaultFileConfig.Server.Bind = %v, want 127.0.0.1", fc.Server.Bind)
	}
	if fc.Server.CookieSecure != nil {
		t.Errorf("DefaultFileConfig.Server.CookieSecure = %v, want nil for auto-detect", formatBoolPtr(fc.Server.CookieSecure))
	}
	if fc.Browsers.Default != BrowserChrome {
		t.Errorf("DefaultFileConfig.Browsers.Default = %v, want %s", fc.Browsers.Default, BrowserChrome)
	}
	if fc.InstanceDefaults.Mode != "headless" {
		t.Errorf("DefaultFileConfig.InstanceDefaults.Mode = %v, want headless", fc.InstanceDefaults.Mode)
	}
	if fc.MultiInstance.Strategy != "always-on" {
		t.Errorf("DefaultFileConfig.MultiInstance.Strategy = %v, want always-on", fc.MultiInstance.Strategy)
	}
	wantSchemes := []string{"ws", "wss", "http", "https"}
	if strings.Join(fc.Security.Attach.AllowSchemes, ",") != strings.Join(wantSchemes, ",") {
		t.Errorf("DefaultFileConfig.Security.Attach.AllowSchemes = %v, want %v", fc.Security.Attach.AllowSchemes, wantSchemes)
	}
	if fc.Security.Attach.Enabled == nil || *fc.Security.Attach.Enabled {
		t.Errorf("DefaultFileConfig.Security.Attach.Enabled = %v, want explicit false", formatBoolPtr(fc.Security.Attach.Enabled))
	}
	if fc.Security.Attach.ForwardProxyAuth == nil || *fc.Security.Attach.ForwardProxyAuth {
		t.Errorf("DefaultFileConfig.Security.Attach.ForwardProxyAuth = %v, want explicit false", formatBoolPtr(fc.Security.Attach.ForwardProxyAuth))
	}
	wantExtensionsDir := defaultExtensionsDir(userConfigDir())
	if len(fc.Browser.ExtensionPaths) != 1 || fc.Browser.ExtensionPaths[0] != wantExtensionsDir {
		t.Errorf("DefaultFileConfig.Browser.ExtensionPaths = %v, want [%q]", fc.Browser.ExtensionPaths, wantExtensionsDir)
	}
	if fc.Security.AllowEvaluate == nil || *fc.Security.AllowEvaluate {
		t.Errorf("DefaultFileConfig.Security.AllowEvaluate = %v, want explicit false", formatBoolPtr(fc.Security.AllowEvaluate))
	}
	if fc.Security.AllowMacro == nil || *fc.Security.AllowMacro {
		t.Errorf("DefaultFileConfig.Security.AllowMacro = %v, want explicit false", formatBoolPtr(fc.Security.AllowMacro))
	}
	if fc.Security.AllowScreencast == nil || *fc.Security.AllowScreencast {
		t.Errorf("DefaultFileConfig.Security.AllowScreencast = %v, want explicit false", formatBoolPtr(fc.Security.AllowScreencast))
	}
	if fc.Security.AllowDownload == nil || *fc.Security.AllowDownload {
		t.Errorf("DefaultFileConfig.Security.AllowDownload = %v, want explicit false", formatBoolPtr(fc.Security.AllowDownload))
	}
	if fc.Security.AllowCookies == nil || *fc.Security.AllowCookies {
		t.Errorf("DefaultFileConfig.Security.AllowCookies = %v, want explicit false", formatBoolPtr(fc.Security.AllowCookies))
	}
	if len(fc.Security.DownloadAllowedDomains) != 0 {
		t.Errorf("DefaultFileConfig.Security.DownloadAllowedDomains = %v, want empty list", fc.Security.DownloadAllowedDomains)
	}
	if fc.Security.DownloadMaxBytes == nil || *fc.Security.DownloadMaxBytes != DefaultDownloadMaxBytes {
		t.Errorf("DefaultFileConfig.Security.DownloadMaxBytes = %v, want %d", formatIntPtr(fc.Security.DownloadMaxBytes), DefaultDownloadMaxBytes)
	}
	if fc.Security.AllowUpload == nil || *fc.Security.AllowUpload {
		t.Errorf("DefaultFileConfig.Security.AllowUpload = %v, want explicit false", formatBoolPtr(fc.Security.AllowUpload))
	}
	if fc.Security.UploadMaxRequestBytes == nil || *fc.Security.UploadMaxRequestBytes != DefaultUploadMaxRequestBytes {
		t.Errorf("DefaultFileConfig.Security.UploadMaxRequestBytes = %v, want %d", formatIntPtr(fc.Security.UploadMaxRequestBytes), DefaultUploadMaxRequestBytes)
	}
	if fc.Security.UploadMaxFiles == nil || *fc.Security.UploadMaxFiles != DefaultUploadMaxFiles {
		t.Errorf("DefaultFileConfig.Security.UploadMaxFiles = %v, want %d", formatIntPtr(fc.Security.UploadMaxFiles), DefaultUploadMaxFiles)
	}
	if fc.Security.UploadMaxFileBytes == nil || *fc.Security.UploadMaxFileBytes != DefaultUploadMaxFileBytes {
		t.Errorf("DefaultFileConfig.Security.UploadMaxFileBytes = %v, want %d", formatIntPtr(fc.Security.UploadMaxFileBytes), DefaultUploadMaxFileBytes)
	}
	if fc.Security.UploadMaxTotalBytes == nil || *fc.Security.UploadMaxTotalBytes != DefaultUploadMaxTotalBytes {
		t.Errorf("DefaultFileConfig.Security.UploadMaxTotalBytes = %v, want %d", formatIntPtr(fc.Security.UploadMaxTotalBytes), DefaultUploadMaxTotalBytes)
	}
	if !fc.Security.IDPI.Enabled {
		t.Errorf("DefaultFileConfig.Security.IDPI.Enabled = %v, want true", fc.Security.IDPI.Enabled)
	}
	if len(fc.Security.AllowedDomains) != 3 || fc.Security.AllowedDomains[0] != "127.0.0.1" {
		t.Errorf("DefaultFileConfig.Security.AllowedDomains = %v, want local-only allowlist", fc.Security.AllowedDomains)
	}
	if !fc.Security.IDPI.StrictMode {
		t.Errorf("DefaultFileConfig.Security.IDPI.StrictMode = %v, want true", fc.Security.IDPI.StrictMode)
	}
	if !fc.Security.IDPI.ScanContent {
		t.Errorf("DefaultFileConfig.Security.IDPI.ScanContent = %v, want true", fc.Security.IDPI.ScanContent)
	}
	if !fc.Security.IDPI.WrapContent {
		t.Errorf("DefaultFileConfig.Security.IDPI.WrapContent = %v, want true", fc.Security.IDPI.WrapContent)
	}
	if fc.Sessions.Dashboard.Persist == nil || !*fc.Sessions.Dashboard.Persist {
		t.Errorf("DefaultFileConfig.Sessions.Dashboard.Persist = %v, want explicit true", formatBoolPtr(fc.Sessions.Dashboard.Persist))
	}
	if fc.Sessions.Dashboard.IdleTimeoutSec == nil || *fc.Sessions.Dashboard.IdleTimeoutSec != 7*24*60*60 {
		t.Errorf("DefaultFileConfig.Sessions.Dashboard.IdleTimeoutSec = %v, want %d", formatIntPtr(fc.Sessions.Dashboard.IdleTimeoutSec), 7*24*60*60)
	}
	if fc.Sessions.Dashboard.MaxLifetimeSec == nil || *fc.Sessions.Dashboard.MaxLifetimeSec != 7*24*60*60 {
		t.Errorf("DefaultFileConfig.Sessions.Dashboard.MaxLifetimeSec = %v, want %d", formatIntPtr(fc.Sessions.Dashboard.MaxLifetimeSec), 7*24*60*60)
	}
	if fc.Sessions.Dashboard.RequireElevation == nil || *fc.Sessions.Dashboard.RequireElevation {
		t.Errorf("DefaultFileConfig.Sessions.Dashboard.RequireElevation = %v, want explicit false", formatBoolPtr(fc.Sessions.Dashboard.RequireElevation))
	}
	if fc.Observability.Activity.StateDir != "" {
		t.Errorf("DefaultFileConfig.Observability.Activity.StateDir = %q, want empty string", fc.Observability.Activity.StateDir)
	}
	if fc.AutoSolver.Enabled == nil || *fc.AutoSolver.Enabled {
		t.Errorf("DefaultFileConfig.AutoSolver.Enabled = %v, want explicit false", formatBoolPtr(fc.AutoSolver.Enabled))
	}
	if fc.AutoSolver.AutoTrigger == nil || !*fc.AutoSolver.AutoTrigger {
		t.Errorf("DefaultFileConfig.AutoSolver.AutoTrigger = %v, want explicit true", formatBoolPtr(fc.AutoSolver.AutoTrigger))
	}
	if fc.AutoSolver.TriggerOnNavigate == nil || !*fc.AutoSolver.TriggerOnNavigate {
		t.Errorf("DefaultFileConfig.AutoSolver.TriggerOnNavigate = %v, want explicit true", formatBoolPtr(fc.AutoSolver.TriggerOnNavigate))
	}
	if fc.AutoSolver.TriggerOnAction == nil || !*fc.AutoSolver.TriggerOnAction {
		t.Errorf("DefaultFileConfig.AutoSolver.TriggerOnAction = %v, want explicit true", formatBoolPtr(fc.AutoSolver.TriggerOnAction))
	}
	if fc.AutoSolver.SolverTimeoutSec == nil || *fc.AutoSolver.SolverTimeoutSec != 30 {
		t.Errorf("DefaultFileConfig.AutoSolver.SolverTimeoutSec = %v, want 30", formatIntPtr(fc.AutoSolver.SolverTimeoutSec))
	}
	if fc.AutoSolver.RetryBaseDelayMs == nil || *fc.AutoSolver.RetryBaseDelayMs != 500 {
		t.Errorf("DefaultFileConfig.AutoSolver.RetryBaseDelayMs = %v, want 500", formatIntPtr(fc.AutoSolver.RetryBaseDelayMs))
	}
	if fc.AutoSolver.RetryMaxDelayMs == nil || *fc.AutoSolver.RetryMaxDelayMs != 10000 {
		t.Errorf("DefaultFileConfig.AutoSolver.RetryMaxDelayMs = %v, want 10000", formatIntPtr(fc.AutoSolver.RetryMaxDelayMs))
	}
	if fc.Observability.Activity.Events.Dashboard == nil || *fc.Observability.Activity.Events.Dashboard {
		t.Errorf("DefaultFileConfig.Observability.Activity.Events.Dashboard = %v, want explicit false", formatBoolPtr(fc.Observability.Activity.Events.Dashboard))
	}
	if fc.Observability.Activity.Events.Server == nil || *fc.Observability.Activity.Events.Server {
		t.Errorf("DefaultFileConfig.Observability.Activity.Events.Server = %v, want explicit false", formatBoolPtr(fc.Observability.Activity.Events.Server))
	}
	if fc.Observability.Activity.Events.Bridge == nil || *fc.Observability.Activity.Events.Bridge {
		t.Errorf("DefaultFileConfig.Observability.Activity.Events.Bridge = %v, want explicit false", formatBoolPtr(fc.Observability.Activity.Events.Bridge))
	}
}

func TestDefaultFileConfigPrefersInstalledCloakBrowser(t *testing.T) {
	setCloakBrowserDiscovery(t, "/opt/cloakbrowser/chrome")
	if got := DefaultFileConfig().Browsers.Default; got != BrowserCloak {
		t.Errorf("DefaultFileConfig().Browsers.Default = %q, want %q when CloakBrowser is installed", got, BrowserCloak)
	}
}

func TestConfigSchemaURLFallsBackToLatestWhenNoPublishedSchemaMatches(t *testing.T) {
	originalPublished := publishedConfigSchemaVersions
	t.Cleanup(func() {
		publishedConfigSchemaVersions = originalPublished
		SetConfigSchemaVersion("dev")
	})

	publishedConfigSchemaVersions = nil

	SetConfigSchemaVersion("1.2.3")
	if got := CurrentConfigSchemaURL(); got != ConfigSchemaURL {
		t.Fatalf("unpublished CurrentConfigSchemaURL() = %q, want latest %q", got, ConfigSchemaURL)
	}
	if fc := DefaultFileConfig(); fc.Schema != ConfigSchemaURL {
		t.Fatalf("DefaultFileConfig().Schema = %q, want latest %q", fc.Schema, ConfigSchemaURL)
	}

	SetConfigSchemaVersion("dev")
	if got := CurrentConfigSchemaURL(); got != ConfigSchemaURL {
		t.Fatalf("dev CurrentConfigSchemaURL() = %q, want %q", got, ConfigSchemaURL)
	}
}

func TestConfigSchemaURLUsesClosestOlderOrEqualPublishedSchema(t *testing.T) {
	originalPublished := publishedConfigSchemaVersions
	t.Cleanup(func() {
		publishedConfigSchemaVersions = originalPublished
		SetConfigSchemaVersion("dev")
	})

	publishedConfigSchemaVersions = []string{"1.2.3", "1.4.0", "2.0.0"}

	SetConfigSchemaVersion("1.2.3")
	wantReleaseURL := "https://raw.githubusercontent.com/pinchtab/pinchtab/v1.2.3/schema/config.json"
	if got := CurrentConfigSchemaURL(); got != wantReleaseURL {
		t.Fatalf("CurrentConfigSchemaURL() = %q, want %q", got, wantReleaseURL)
	}
	if fc := DefaultFileConfig(); fc.Schema != wantReleaseURL {
		t.Fatalf("DefaultFileConfig().Schema = %q, want %q", fc.Schema, wantReleaseURL)
	}

	SetConfigSchemaVersion("1.3.0")
	wantClosestOlderURL := "https://raw.githubusercontent.com/pinchtab/pinchtab/v1.2.3/schema/config.json"
	if got := CurrentConfigSchemaURL(); got != wantClosestOlderURL {
		t.Fatalf("closest older CurrentConfigSchemaURL() = %q, want %q", got, wantClosestOlderURL)
	}

	SetConfigSchemaVersion("1.1.0")
	if got := CurrentConfigSchemaURL(); got != ConfigSchemaURL {
		t.Fatalf("older than first published CurrentConfigSchemaURL() = %q, want latest %q", got, ConfigSchemaURL)
	}

	SetConfigSchemaVersion("2.1.0")
	wantLatestPublishedURL := "https://raw.githubusercontent.com/pinchtab/pinchtab/v2.0.0/schema/config.json"
	if got := CurrentConfigSchemaURL(); got != wantLatestPublishedURL {
		t.Fatalf("newer than published CurrentConfigSchemaURL() = %q, want %q", got, wantLatestPublishedURL)
	}
}

func TestFileConfigFromRuntime_PreservesBrowserTargets(t *testing.T) {
	quota := 256
	disableStealth := true
	cfg := &RuntimeConfig{
		DefaultBrowser: BrowserChrome,
		DefaultTarget:  "chrome-local",
		FallbackOrder:  []string{"cloak-primary"},
		Targets: BrowserTargetsConfig{
			"chrome-local": {
				Provider:   BrowserChrome,
				Binary:     "/usr/bin/chrome",
				ExtraFlags: "--ash-no-nudges",
				Proxy: BrowserProxyConfig{
					Server:     "http://proxy.example:8080",
					BypassList: []string{"localhost"},
					Geo:        &BrowserProxyGeoConfig{Timezone: "UTC"},
				},
			},
			"cloak-primary": {
				Provider: BrowserCloak,
				Binary:   "/opt/cloak/chrome",
				Cloak: CloakBrowserConfig{
					StorageQuotaMB:            &quota,
					DisableDefaultStealthArgs: &disableStealth,
				},
			},
		},
	}

	fc := FileConfigFromRuntime(cfg)
	if fc.Browser.DefaultTarget != "chrome-local" {
		t.Fatalf("DefaultTarget = %q, want chrome-local", fc.Browser.DefaultTarget)
	}
	if got := strings.Join(fc.Browser.FallbackOrder, ","); got != "cloak-primary" {
		t.Fatalf("FallbackOrder = %q, want cloak-primary", got)
	}
	if len(fc.Browser.Targets) != 2 {
		t.Fatalf("Targets len = %d, want 2", len(fc.Browser.Targets))
	}
	if fc.Browser.Targets["chrome-local"].Proxy.BypassList[0] != "localhost" {
		t.Fatalf("proxy bypass not preserved: %+v", fc.Browser.Targets["chrome-local"].Proxy)
	}
	if fc.Browser.Targets["cloak-primary"].Cloak.StorageQuotaMB == nil ||
		*fc.Browser.Targets["cloak-primary"].Cloak.StorageQuotaMB != 256 {
		t.Fatalf("cloak target quota not preserved: %+v", fc.Browser.Targets["cloak-primary"].Cloak)
	}

	fc.Browser.FallbackOrder[0] = "mutated"
	target := fc.Browser.Targets["chrome-local"]
	target.Proxy.BypassList[0] = "mutated"
	fc.Browser.Targets["chrome-local"] = target
	if cfg.FallbackOrder[0] != "cloak-primary" {
		t.Fatalf("runtime fallbackOrder shared with file config: %+v", cfg.FallbackOrder)
	}
	if cfg.Targets["chrome-local"].Proxy.BypassList[0] != "localhost" {
		t.Fatalf("runtime target proxy shared with file config: %+v", cfg.Targets["chrome-local"].Proxy)
	}
}

func TestIsLegacyConfig(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		isLegacy bool
	}{
		{
			name:     "nested format with server",
			json:     `{"server": {"port": "9867"}}`,
			isLegacy: false,
		},
		{
			name:     "nested format with instanceDefaults",
			json:     `{"instanceDefaults": {"mode": "headless"}}`,
			isLegacy: false,
		},
		{
			name:     "nested format with security.attach",
			json:     `{"security": {"attach": {"enabled": true}}}`,
			isLegacy: false,
		},
		{
			name:     "legacy format with port",
			json:     `{"port": "9867"}`,
			isLegacy: true,
		},
		{
			name:     "legacy format with headless",
			json:     `{"headless": true}`,
			isLegacy: true,
		},
		{
			name:     "empty object",
			json:     `{}`,
			isLegacy: false,
		},
		{
			name:     "mixed - nested wins",
			json:     `{"server": {"port": "8888"}, "port": "7777"}`,
			isLegacy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLegacyConfig([]byte(tt.json))
			if got != tt.isLegacy {
				t.Errorf("isLegacyConfig(%s) = %v, want %v", tt.json, got, tt.isLegacy)
			}
		})
	}
}

func TestConvertLegacyConfig(t *testing.T) {
	h := false
	maxTabs := 25
	lc := &legacyFileConfig{
		Port:          "7777",
		Headless:      &h,
		MaxTabs:       &maxTabs,
		AllowEvaluate: boolPtr(true),
		TimeoutSec:    45,
		NavigateSec:   90,
	}

	fc := convertLegacyConfig(lc)

	if fc.Server.Port != "7777" {
		t.Errorf("converted Server.Port = %v, want 7777", fc.Server.Port)
	}
	if fc.InstanceDefaults.Mode != "headed" {
		t.Errorf("converted InstanceDefaults.Mode = %v, want headed", fc.InstanceDefaults.Mode)
	}
	if *fc.InstanceDefaults.MaxTabs != 25 {
		t.Errorf("converted InstanceDefaults.MaxTabs = %v, want 25", *fc.InstanceDefaults.MaxTabs)
	}
	if *fc.Security.AllowEvaluate != true {
		t.Errorf("converted Security.AllowEvaluate = %v, want true", *fc.Security.AllowEvaluate)
	}
	if fc.Timeouts.ActionSec != 45 {
		t.Errorf("converted Timeouts.ActionSec = %v, want 45", fc.Timeouts.ActionSec)
	}
	if fc.Timeouts.NavigateSec != 90 {
		t.Errorf("converted Timeouts.NavigateSec = %v, want 90", fc.Timeouts.NavigateSec)
	}
}

func TestTabPolicyDefaultsFromRuntime(t *testing.T) {
	if got := tabPolicyDefaultsFromRuntime(&RuntimeConfig{
		TabLifecyclePolicy: "keep",
		TabCloseDelay:      5 * time.Minute,
	}); got != nil {
		t.Fatalf("default keep policy should not be emitted; got %#v", got)
	}

	got := tabPolicyDefaultsFromRuntime(&RuntimeConfig{
		TabLifecyclePolicy: "close_idle",
		TabCloseDelay:      5 * time.Minute,
	})
	if got == nil || got.Lifecycle != "close_idle" || got.CloseDelaySec != nil {
		t.Fatalf("close_idle implicit-delay policy = %#v, want lifecycle=close_idle without delay", got)
	}

	got = tabPolicyDefaultsFromRuntime(&RuntimeConfig{
		TabLifecyclePolicy: "close_idle",
		TabCloseDelay:      90 * time.Second,
	})
	if got == nil || got.Lifecycle != "close_idle" || got.CloseDelaySec == nil || *got.CloseDelaySec != 90 {
		t.Fatalf("custom close_idle policy = %#v, want lifecycle=close_idle closeDelaySec=90", got)
	}
}

func TestDefaultFileConfigJSON(t *testing.T) {
	fc := DefaultFileConfig()
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal DefaultFileConfig: %v", err)
	}

	var parsed FileConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DefaultFileConfig output: %v", err)
	}

	if !strings.HasPrefix(string(data), "{\n  \"$schema\": ") {
		t.Fatalf("DefaultFileConfig JSON should start with $schema, got:\n%s", data)
	}
	if parsed.Schema != CurrentConfigSchemaURL() {
		t.Errorf("round-trip Schema = %q, want %q", parsed.Schema, CurrentConfigSchemaURL())
	}
	if parsed.Server.Port != "9867" {
		t.Errorf("round-trip Server.Port = %v, want 9867", parsed.Server.Port)
	}
	if parsed.InstanceDefaults.Mode != "headless" {
		t.Errorf("round-trip InstanceDefaults.Mode = %v, want headless", parsed.InstanceDefaults.Mode)
	}
	if parsed.Security.AllowEvaluate == nil || *parsed.Security.AllowEvaluate {
		t.Errorf("round-trip Security.AllowEvaluate = %v, want explicit false", formatBoolPtr(parsed.Security.AllowEvaluate))
	}
	wantExtensionsDir := defaultExtensionsDir(userConfigDir())
	if len(parsed.Browser.ExtensionPaths) != 1 || parsed.Browser.ExtensionPaths[0] != wantExtensionsDir {
		t.Errorf("round-trip Browser.ExtensionPaths = %v, want [%q]", parsed.Browser.ExtensionPaths, wantExtensionsDir)
	}
	if parsed.Security.AllowMacro == nil || *parsed.Security.AllowMacro {
		t.Errorf("round-trip Security.AllowMacro = %v, want explicit false", formatBoolPtr(parsed.Security.AllowMacro))
	}
	if parsed.Security.AllowScreencast == nil || *parsed.Security.AllowScreencast {
		t.Errorf("round-trip Security.AllowScreencast = %v, want explicit false", formatBoolPtr(parsed.Security.AllowScreencast))
	}
	if parsed.Security.AllowDownload == nil || *parsed.Security.AllowDownload {
		t.Errorf("round-trip Security.AllowDownload = %v, want explicit false", formatBoolPtr(parsed.Security.AllowDownload))
	}
	if parsed.Security.AllowCookies == nil || *parsed.Security.AllowCookies {
		t.Errorf("round-trip Security.AllowCookies = %v, want explicit false", formatBoolPtr(parsed.Security.AllowCookies))
	}
	if len(parsed.Security.DownloadAllowedDomains) != 0 {
		t.Errorf("round-trip Security.DownloadAllowedDomains = %v, want empty list", parsed.Security.DownloadAllowedDomains)
	}
	if parsed.Security.DownloadMaxBytes == nil || *parsed.Security.DownloadMaxBytes != DefaultDownloadMaxBytes {
		t.Errorf("round-trip Security.DownloadMaxBytes = %v, want %d", formatIntPtr(parsed.Security.DownloadMaxBytes), DefaultDownloadMaxBytes)
	}
	if parsed.Security.AllowUpload == nil || *parsed.Security.AllowUpload {
		t.Errorf("round-trip Security.AllowUpload = %v, want explicit false", formatBoolPtr(parsed.Security.AllowUpload))
	}
	if parsed.Security.UploadMaxRequestBytes == nil || *parsed.Security.UploadMaxRequestBytes != DefaultUploadMaxRequestBytes {
		t.Errorf("round-trip Security.UploadMaxRequestBytes = %v, want %d", formatIntPtr(parsed.Security.UploadMaxRequestBytes), DefaultUploadMaxRequestBytes)
	}
	if parsed.Security.UploadMaxFiles == nil || *parsed.Security.UploadMaxFiles != DefaultUploadMaxFiles {
		t.Errorf("round-trip Security.UploadMaxFiles = %v, want %d", formatIntPtr(parsed.Security.UploadMaxFiles), DefaultUploadMaxFiles)
	}
	if parsed.Security.UploadMaxFileBytes == nil || *parsed.Security.UploadMaxFileBytes != DefaultUploadMaxFileBytes {
		t.Errorf("round-trip Security.UploadMaxFileBytes = %v, want %d", formatIntPtr(parsed.Security.UploadMaxFileBytes), DefaultUploadMaxFileBytes)
	}
	if parsed.Security.UploadMaxTotalBytes == nil || *parsed.Security.UploadMaxTotalBytes != DefaultUploadMaxTotalBytes {
		t.Errorf("round-trip Security.UploadMaxTotalBytes = %v, want %d", formatIntPtr(parsed.Security.UploadMaxTotalBytes), DefaultUploadMaxTotalBytes)
	}
	if parsed.Security.Attach.Enabled == nil || *parsed.Security.Attach.Enabled {
		t.Errorf("round-trip Security.Attach.Enabled = %v, want explicit false", formatBoolPtr(parsed.Security.Attach.Enabled))
	}
	if parsed.Security.Attach.ForwardProxyAuth == nil || *parsed.Security.Attach.ForwardProxyAuth {
		t.Errorf("round-trip Security.Attach.ForwardProxyAuth = %v, want explicit false", formatBoolPtr(parsed.Security.Attach.ForwardProxyAuth))
	}
	if parsed.Observability.Activity.StateDir != "" {
		t.Errorf("round-trip Observability.Activity.StateDir = %q, want empty string", parsed.Observability.Activity.StateDir)
	}
	if parsed.Observability.Activity.Events.Dashboard == nil || *parsed.Observability.Activity.Events.Dashboard {
		t.Errorf("round-trip Observability.Activity.Events.Dashboard = %v, want explicit false", formatBoolPtr(parsed.Observability.Activity.Events.Dashboard))
	}
	if parsed.Observability.Activity.Events.Server == nil || *parsed.Observability.Activity.Events.Server {
		t.Errorf("round-trip Observability.Activity.Events.Server = %v, want explicit false", formatBoolPtr(parsed.Observability.Activity.Events.Server))
	}
	if !parsed.Security.IDPI.Enabled {
		t.Errorf("round-trip Security.IDPI.Enabled = %v, want true", parsed.Security.IDPI.Enabled)
	}
	if len(parsed.Security.AllowedDomains) != 3 || parsed.Security.AllowedDomains[0] != "127.0.0.1" {
		t.Errorf("round-trip Security.AllowedDomains = %v, want local-only allowlist", parsed.Security.AllowedDomains)
	}
	if !parsed.Security.IDPI.StrictMode {
		t.Errorf("round-trip Security.IDPI.StrictMode = %v, want true", parsed.Security.IDPI.StrictMode)
	}
	if !parsed.Security.IDPI.ScanContent {
		t.Errorf("round-trip Security.IDPI.ScanContent = %v, want true", parsed.Security.IDPI.ScanContent)
	}
	if !parsed.Security.IDPI.WrapContent {
		t.Errorf("round-trip Security.IDPI.WrapContent = %v, want true", parsed.Security.IDPI.WrapContent)
	}
	if parsed.Security.IDPI.ShieldThreshold != 0 {
		t.Errorf("round-trip Security.IDPI.ShieldThreshold = %d, want 0", parsed.Security.IDPI.ShieldThreshold)
	}
}

func TestFileConfigJSONPreservesExplicitZeroValues(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Server.Bind = ""
	fc.Browser.ExtensionPaths = []string{}
	fc.InstanceDefaults.UserAgent = ""
	fc.Security.IDPI.StrictMode = false
	fc.Security.AllowedDomains = []string{}
	fc.Security.IDPI.CustomPatterns = []string{}
	fc.Security.IDPI.ShieldThreshold = 30

	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("json.Marshal(FileConfig) error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal(FileConfig JSON) error = %v", err)
	}

	server := raw["server"].(map[string]any)
	if bind, ok := server["bind"]; !ok || bind != "" {
		t.Fatalf("server.bind = %#v, want explicit empty string", bind)
	}

	browser := raw["browser"].(map[string]any)
	if _, ok := browser["provider"]; ok {
		t.Fatal("browser.provider should not be emitted in JSON (deprecated in favor of browsers.default)")
	}
	if browsersRaw, ok := raw["browsers"]; !ok {
		t.Fatal("browsers block missing from JSON")
	} else if browsersMap, ok := browsersRaw.(map[string]any); !ok {
		t.Fatal("browsers block is not an object")
	} else if def, ok := browsersMap["default"]; !ok || def != BrowserChrome {
		t.Fatalf("browsers.default = %#v, want chrome", def)
	}
	if _, ok := browser["cloak"]; ok {
		t.Fatal("browser.cloak should not be emitted for default chrome provider")
	}
	if ext, ok := browser["extensionPaths"]; !ok {
		t.Fatal("browser.extensionPaths missing from JSON")
	} else if items, ok := ext.([]any); !ok || len(items) != 0 {
		t.Fatalf("browser.extensionPaths = %#v, want explicit empty list", ext)
	}

	security := raw["security"].(map[string]any)
	idpi := security["idpi"].(map[string]any)
	if strictMode, ok := idpi["strictMode"]; !ok || strictMode != false {
		t.Fatalf("security.idpi.strictMode = %#v, want explicit false", strictMode)
	}
	if allowedDomains, ok := security["allowedDomains"]; !ok {
		t.Fatal("security.allowedDomains missing from JSON")
	} else if items, ok := allowedDomains.([]any); !ok || len(items) != 0 {
		t.Fatalf("security.allowedDomains = %#v, want explicit empty list", allowedDomains)
	}
	if _, ok := idpi["allowedDomains"]; ok {
		t.Fatal("security.idpi.allowedDomains should not be emitted in JSON")
	}
	if raw, ok := idpi["shieldThreshold"]; !ok || int(raw.(float64)) != 30 {
		t.Fatalf("security.idpi.shieldThreshold = %#v, want 30", raw)
	}
	if downloadAllowedDomains, ok := security["downloadAllowedDomains"]; !ok {
		t.Fatal("security.downloadAllowedDomains missing from JSON")
	} else if items, ok := downloadAllowedDomains.([]any); !ok || len(items) != 0 {
		t.Fatalf("security.downloadAllowedDomains = %#v, want explicit empty list", downloadAllowedDomains)
	}
	if raw, ok := security["downloadMaxBytes"]; !ok || int(raw.(float64)) != DefaultDownloadMaxBytes {
		t.Fatalf("security.downloadMaxBytes = %#v, want %d", raw, DefaultDownloadMaxBytes)
	}
	if raw, ok := security["uploadMaxRequestBytes"]; !ok || int(raw.(float64)) != DefaultUploadMaxRequestBytes {
		t.Fatalf("security.uploadMaxRequestBytes = %#v, want %d", raw, DefaultUploadMaxRequestBytes)
	}
	if raw, ok := security["uploadMaxFiles"]; !ok || int(raw.(float64)) != DefaultUploadMaxFiles {
		t.Fatalf("security.uploadMaxFiles = %#v, want %d", raw, DefaultUploadMaxFiles)
	}
	if raw, ok := security["uploadMaxFileBytes"]; !ok || int(raw.(float64)) != DefaultUploadMaxFileBytes {
		t.Fatalf("security.uploadMaxFileBytes = %#v, want %d", raw, DefaultUploadMaxFileBytes)
	}
	if raw, ok := security["uploadMaxTotalBytes"]; !ok || int(raw.(float64)) != DefaultUploadMaxTotalBytes {
		t.Fatalf("security.uploadMaxTotalBytes = %#v, want %d", raw, DefaultUploadMaxTotalBytes)
	}
}

func TestFileConfigJSONEmitsCloakBrowserConfig(t *testing.T) {
	disableDefaultStealthArgs := true
	fc := DefaultFileConfig()
	fc.Browser.Provider = BrowserCloak
	fc.Browser.BrowserBinary = "/opt/cloakbrowser/chrome"
	fc.Browser.Cloak = CloakBrowserConfig{
		FingerprintSeed:           "42069",
		Platform:                  "windows",
		DisableDefaultStealthArgs: &disableDefaultStealthArgs,
	}

	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("json.Marshal(FileConfig) error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal(FileConfig JSON) error = %v", err)
	}
	browser := raw["browser"].(map[string]any)
	cloak, ok := browser["cloak"].(map[string]any)
	if !ok {
		t.Fatal("browser.cloak missing from JSON")
	}
	if cloak["fingerprintSeed"] != "42069" {
		t.Fatalf("browser.cloak.fingerprintSeed = %#v, want 42069", cloak["fingerprintSeed"])
	}
	if cloak["platform"] != "windows" {
		t.Fatalf("browser.cloak.platform = %#v, want windows", cloak["platform"])
	}
}

func TestFileConfigJSONEmitsExplicitCloakFalseOnChromeProvider(t *testing.T) {
	disableDefaultStealthArgs := false
	fc := DefaultFileConfig()
	fc.Browser.Provider = BrowserChrome
	fc.Browser.Cloak = CloakBrowserConfig{DisableDefaultStealthArgs: &disableDefaultStealthArgs}

	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("json.Marshal(FileConfig) error = %v", err)
	}
	if !strings.Contains(string(data), `"disableDefaultStealthArgs":false`) {
		t.Fatalf("explicit false cloak setting not preserved in JSON: %s", data)
	}
}

func TestFileConfigFromRuntimeEmitsCloakStorageQuotaZeroForCloakProvider(t *testing.T) {
	fc := FileConfigFromRuntime(&RuntimeConfig{
		DefaultBrowser: BrowserCloak,
		BrowserBinary:  "/opt/cloakbrowser/chrome",
		Cloak: CloakBrowserRuntimeConfig{
			StorageQuotaMB:            0,
			DisableDefaultStealthArgs: true,
		},
	})
	if fc.Browser.Cloak.StorageQuotaMB == nil || *fc.Browser.Cloak.StorageQuotaMB != 0 {
		t.Fatalf("StorageQuotaMB = %v, want explicit 0", fc.Browser.Cloak.StorageQuotaMB)
	}
}

// An unknown browsers.default must be written verbatim (config load keeps it raw
// so the load-time warning + chrome fallback still fire); marshal must not
// normalize it to chrome. An empty value falls back to chrome.
func TestFileConfigFromRuntime_DefaultBrowserVerbatim(t *testing.T) {
	if got := FileConfigFromRuntime(&RuntimeConfig{DefaultBrowser: "foo"}).Browsers.Default; got != "foo" {
		t.Errorf("unknown DefaultBrowser = %q, want verbatim \"foo\" (not normalized)", got)
	}
	if got := FileConfigFromRuntime(&RuntimeConfig{DefaultBrowser: ""}).Browsers.Default; got != BrowserChrome {
		t.Errorf("empty DefaultBrowser = %q, want chrome fallback", got)
	}
	if got := FileConfigFromRuntime(&RuntimeConfig{DefaultBrowser: BrowserCloak}).Browsers.Default; got != BrowserCloak {
		t.Errorf("known DefaultBrowser = %q, want cloak", got)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
