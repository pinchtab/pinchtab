package config

import (
	"encoding/json"
	"testing"
)

func TestDefaultFileConfig(t *testing.T) {
	fc := DefaultFileConfig()
	if fc.Server.Port != "9867" {
		t.Errorf("DefaultFileConfig.Server.Port = %v, want 9867", fc.Server.Port)
	}
	if fc.Server.Bind != "127.0.0.1" {
		t.Errorf("DefaultFileConfig.Server.Bind = %v, want 127.0.0.1", fc.Server.Bind)
	}
	if fc.Server.CookieSecure != nil {
		t.Errorf("DefaultFileConfig.Server.CookieSecure = %v, want nil for auto-detect", formatBoolPtr(fc.Server.CookieSecure))
	}
	if fc.InstanceDefaults.Mode != "headless" {
		t.Errorf("DefaultFileConfig.InstanceDefaults.Mode = %v, want headless", fc.InstanceDefaults.Mode)
	}
	if fc.MultiInstance.Strategy != "always-on" {
		t.Errorf("DefaultFileConfig.MultiInstance.Strategy = %v, want always-on", fc.MultiInstance.Strategy)
	}
	if len(fc.Security.Attach.AllowSchemes) != 2 || fc.Security.Attach.AllowSchemes[0] != "ws" || fc.Security.Attach.AllowSchemes[1] != "wss" {
		t.Errorf("DefaultFileConfig.Security.Attach.AllowSchemes = %v, want [ws wss]", fc.Security.Attach.AllowSchemes)
	}
	if fc.Security.Attach.Enabled == nil || *fc.Security.Attach.Enabled {
		t.Errorf("DefaultFileConfig.Security.Attach.Enabled = %v, want explicit false", formatBoolPtr(fc.Security.Attach.Enabled))
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
	if len(fc.Security.IDPI.AllowedDomains) != 3 || fc.Security.IDPI.AllowedDomains[0] != "127.0.0.1" {
		t.Errorf("DefaultFileConfig.Security.IDPI.AllowedDomains = %v, want local-only allowlist", fc.Security.IDPI.AllowedDomains)
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
}

// TestIsLegacyConfig tests the format detection logic.
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

// TestConvertLegacyConfig tests the legacy to nested conversion.
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

// TestDefaultFileConfigJSON tests that DefaultFileConfig serializes correctly.
func TestDefaultFileConfigJSON(t *testing.T) {
	fc := DefaultFileConfig()
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal DefaultFileConfig: %v", err)
	}

	// Verify it can be parsed back
	var parsed FileConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DefaultFileConfig output: %v", err)
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
	if parsed.Security.AllowMacro == nil || *parsed.Security.AllowMacro {
		t.Errorf("round-trip Security.AllowMacro = %v, want explicit false", formatBoolPtr(parsed.Security.AllowMacro))
	}
	if parsed.Security.AllowScreencast == nil || *parsed.Security.AllowScreencast {
		t.Errorf("round-trip Security.AllowScreencast = %v, want explicit false", formatBoolPtr(parsed.Security.AllowScreencast))
	}
	if parsed.Security.AllowDownload == nil || *parsed.Security.AllowDownload {
		t.Errorf("round-trip Security.AllowDownload = %v, want explicit false", formatBoolPtr(parsed.Security.AllowDownload))
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
	if parsed.Observability.Activity.StateDir != "" {
		t.Errorf("round-trip Observability.Activity.StateDir = %q, want empty string", parsed.Observability.Activity.StateDir)
	}
	if !parsed.Security.IDPI.Enabled {
		t.Errorf("round-trip Security.IDPI.Enabled = %v, want true", parsed.Security.IDPI.Enabled)
	}
	if len(parsed.Security.IDPI.AllowedDomains) != 3 || parsed.Security.IDPI.AllowedDomains[0] != "127.0.0.1" {
		t.Errorf("round-trip Security.IDPI.AllowedDomains = %v, want local-only allowlist", parsed.Security.IDPI.AllowedDomains)
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
	fc.Security.IDPI.AllowedDomains = []string{}
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
	if allowedDomains, ok := idpi["allowedDomains"]; !ok {
		t.Fatal("security.idpi.allowedDomains missing from JSON")
	} else if items, ok := allowedDomains.([]any); !ok || len(items) != 0 {
		t.Fatalf("security.idpi.allowedDomains = %#v, want explicit empty list", allowedDomains)
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

func boolPtr(b bool) *bool {
	return &b
}
