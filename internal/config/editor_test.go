package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetConfigValue_ServerFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"server.port", "8080", func(fc *FileConfig) bool { return fc.Server.Port == "8080" }, false},
		{"server.bind", "0.0.0.0", func(fc *FileConfig) bool { return fc.Server.Bind == "0.0.0.0" }, false},
		{"server.token", "secret", func(fc *FileConfig) bool { return fc.Server.Token == "secret" }, false},
		{"server.stateDir", "/tmp/state", func(fc *FileConfig) bool { return fc.Server.StateDir == "/tmp/state" }, false},
		{"server.unknown", "value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_BrowserAndInstanceDefaultsFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"browser.version", "144.0.7559.133", func(fc *FileConfig) bool { return fc.Browser.ChromeVersion == "144.0.7559.133" }, false},
		{"browser.binary", "/tmp/chrome", func(fc *FileConfig) bool { return fc.Browser.ChromeBinary == "/tmp/chrome" }, false},
		{"instanceDefaults.mode", "headed", func(fc *FileConfig) bool { return fc.InstanceDefaults.Mode == "headed" }, false},
		{"instanceDefaults.maxTabs", "50", func(fc *FileConfig) bool { return *fc.InstanceDefaults.MaxTabs == 50 }, false},
		{"instanceDefaults.stealthLevel", "full", func(fc *FileConfig) bool { return fc.InstanceDefaults.StealthLevel == "full" }, false},
		{"instanceDefaults.tabEvictionPolicy", "close_lru", func(fc *FileConfig) bool { return fc.InstanceDefaults.TabEvictionPolicy == "close_lru" }, false},
		{"instanceDefaults.blockAds", "yes", func(fc *FileConfig) bool { return *fc.InstanceDefaults.BlockAds == true }, false},
		{"profiles.baseDir", "/tmp/profiles", func(fc *FileConfig) bool { return fc.Profiles.BaseDir == "/tmp/profiles" }, false},
		{"instanceDefaults.noRestore", "maybe", nil, true},
		{"instanceDefaults.maxTabs", "many", nil, true},
		{"instanceDefaults.unknown", "value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_SecurityFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"security.allowEvaluate", "true", func(fc *FileConfig) bool { return *fc.Security.AllowEvaluate == true }, false},
		{"security.allowMacro", "1", func(fc *FileConfig) bool { return *fc.Security.AllowMacro == true }, false},
		{"security.allowScreencast", "false", func(fc *FileConfig) bool { return *fc.Security.AllowScreencast == false }, false},
		{"security.allowDownload", "on", func(fc *FileConfig) bool { return *fc.Security.AllowDownload == true }, false},
		{"security.allowUpload", "off", func(fc *FileConfig) bool { return *fc.Security.AllowUpload == false }, false},
		{"security.allowEvaluate", "maybe", nil, true},
		{"security.unknown", "true", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_MultiInstanceFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"multiInstance.strategy", "explicit", func(fc *FileConfig) bool { return fc.MultiInstance.Strategy == "explicit" }, false},
		{"multiInstance.allocationPolicy", "round_robin", func(fc *FileConfig) bool { return fc.MultiInstance.AllocationPolicy == "round_robin" }, false},
		{"multiInstance.disableLocalLaunch", "true", func(fc *FileConfig) bool {
			return fc.MultiInstance.DisableLocalLaunch != nil && *fc.MultiInstance.DisableLocalLaunch
		}, false},
		{"multiInstance.instancePortStart", "9900", func(fc *FileConfig) bool { return *fc.MultiInstance.InstancePortStart == 9900 }, false},
		{"multiInstance.restart.maxRestarts", "12", func(fc *FileConfig) bool {
			return fc.MultiInstance.Restart.MaxRestarts != nil && *fc.MultiInstance.Restart.MaxRestarts == 12
		}, false},
		{"multiInstance.restart.initBackoffSec", "3", func(fc *FileConfig) bool {
			return fc.MultiInstance.Restart.InitBackoffSec != nil && *fc.MultiInstance.Restart.InitBackoffSec == 3
		}, false},
		{"multiInstance.unknown", "value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_AttachFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"security.attach.enabled", "true", func(fc *FileConfig) bool { return fc.Security.Attach.Enabled != nil && *fc.Security.Attach.Enabled }, false},
		{"security.attach.allowHosts", "localhost, chrome.internal", func(fc *FileConfig) bool {
			return len(fc.Security.Attach.AllowHosts) == 2 && fc.Security.Attach.AllowHosts[1] == "chrome.internal"
		}, false},
		{"security.attach.allowSchemes", "ws,wss", func(fc *FileConfig) bool {
			return len(fc.Security.Attach.AllowSchemes) == 2 && fc.Security.Attach.AllowSchemes[0] == "ws"
		}, false},
		{"security.attach.enabled", "maybe", nil, true},
		{"security.attach.unknown", "value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_IDPIFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"security.idpi.enabled", "true", func(fc *FileConfig) bool { return fc.Security.IDPI.Enabled }, false},
		{"security.idpi.allowedDomains", "localhost, example.com", func(fc *FileConfig) bool {
			return len(fc.Security.IDPI.AllowedDomains) == 2 && fc.Security.IDPI.AllowedDomains[1] == "example.com"
		}, false},
		{"security.idpi.strictMode", "false", func(fc *FileConfig) bool { return !fc.Security.IDPI.StrictMode }, false},
		{"security.idpi.scanContent", "true", func(fc *FileConfig) bool { return fc.Security.IDPI.ScanContent }, false},
		{"security.idpi.wrapContent", "true", func(fc *FileConfig) bool { return fc.Security.IDPI.WrapContent }, false},
		{"security.idpi.customPatterns", "ignore previous instructions, exfiltrate data", func(fc *FileConfig) bool {
			return len(fc.Security.IDPI.CustomPatterns) == 2 && fc.Security.IDPI.CustomPatterns[0] == "ignore previous instructions"
		}, false},
		{"security.idpi.enabled", "maybe", nil, true},
		{"security.idpi.unknown", "value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_TimeoutsFields(t *testing.T) {
	tests := []struct {
		path    string
		value   string
		check   func(*FileConfig) bool
		wantErr bool
	}{
		{"timeouts.actionSec", "60", func(fc *FileConfig) bool { return fc.Timeouts.ActionSec == 60 }, false},
		{"timeouts.navigateSec", "120", func(fc *FileConfig) bool { return fc.Timeouts.NavigateSec == 120 }, false},
		{"timeouts.shutdownSec", "30", func(fc *FileConfig) bool { return fc.Timeouts.ShutdownSec == 30 }, false},
		{"timeouts.waitNavMs", "2000", func(fc *FileConfig) bool { return fc.Timeouts.WaitNavMs == 2000 }, false},
		{"timeouts.actionSec", "fast", nil, true},
		{"timeouts.unknown", "10", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetConfigValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check(fc) {
				t.Errorf("SetConfigValue() did not set value correctly")
			}
		})
	}
}

func TestSetConfigValue_InvalidPaths(t *testing.T) {
	tests := []string{
		"port",          // missing section
		"",              // empty
		"unknown.field", // unknown section
		"server",        // missing field
		"a.b.c",         // too many parts (we only split on first .)
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			fc := &FileConfig{}
			err := SetConfigValue(fc, path, "value")
			if err == nil {
				t.Errorf("SetConfigValue(%q) should have failed", path)
			}
		})
	}
}

func TestPatchConfigJSON(t *testing.T) {
	fc := &FileConfig{
		Server: ServerConfig{
			Port: "9867",
			Bind: "127.0.0.1",
		},
		InstanceDefaults: InstanceDefaultsConfig{
			StealthLevel: "light",
		},
	}

	// Patch to change port and add token
	patch := `{"server": {"port": "8080", "token": "secret"}}`
	if err := PatchConfigJSON(fc, patch); err != nil {
		t.Fatalf("PatchConfigJSON() error = %v", err)
	}

	if fc.Server.Port != "8080" {
		t.Errorf("port = %v, want 8080", fc.Server.Port)
	}
	if fc.Server.Token != "secret" {
		t.Errorf("token = %v, want secret", fc.Server.Token)
	}
	// Bind should be preserved
	if fc.Server.Bind != "127.0.0.1" {
		t.Errorf("bind = %v, want 127.0.0.1 (should be preserved)", fc.Server.Bind)
	}
	// InstanceDefaults.StealthLevel should be preserved
	if fc.InstanceDefaults.StealthLevel != "light" {
		t.Errorf("stealthLevel = %v, want light (should be preserved)", fc.InstanceDefaults.StealthLevel)
	}
}

func TestPatchConfigJSON_NestedMerge(t *testing.T) {
	fc := &FileConfig{
		InstanceDefaults: InstanceDefaultsConfig{
			StealthLevel:      "light",
			TabEvictionPolicy: "reject",
		},
	}

	// Patch instanceDefaults section, should merge not replace
	patch := `{"instanceDefaults": {"stealthLevel": "full"}}`
	if err := PatchConfigJSON(fc, patch); err != nil {
		t.Fatalf("PatchConfigJSON() error = %v", err)
	}

	if fc.InstanceDefaults.StealthLevel != "full" {
		t.Errorf("stealthLevel = %v, want full", fc.InstanceDefaults.StealthLevel)
	}
	// tabEvictionPolicy should be preserved
	if fc.InstanceDefaults.TabEvictionPolicy != "reject" {
		t.Errorf("tabEvictionPolicy = %v, want reject (should be preserved)", fc.InstanceDefaults.TabEvictionPolicy)
	}
}

func TestPatchConfigJSON_InvalidJSON(t *testing.T) {
	fc := &FileConfig{}
	err := PatchConfigJSON(fc, "not json")
	if err == nil {
		t.Error("PatchConfigJSON() should fail on invalid JSON")
	}
}

func TestLoadAndSaveFileConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	// Load (should return empty config for non-existent file)
	fc, path, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if path != configPath {
		t.Errorf("path = %v, want %v", path, configPath)
	}

	// Modify
	fc.Server.Port = "8080"
	fc.InstanceDefaults.StealthLevel = "full"

	// Save
	if err := SaveFileConfig(fc, path); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	// Load again
	fc2, _, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() second time error = %v", err)
	}

	if fc2.Server.Port != "8080" {
		t.Errorf("loaded port = %v, want 8080", fc2.Server.Port)
	}
	if fc2.InstanceDefaults.StealthLevel != "full" {
		t.Errorf("loaded stealthLevel = %v, want full", fc2.InstanceDefaults.StealthLevel)
	}
}

func TestLoadAndSaveFileConfigPreservesExplicitZeroValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	fc := DefaultFileConfig()
	fc.Server.Bind = ""
	fc.Server.Token = ""
	fc.Browser.ExtensionPaths = []string{}
	fc.InstanceDefaults.UserAgent = ""
	fc.Security.IDPI.StrictMode = false
	fc.Security.IDPI.AllowedDomains = []string{}
	fc.Security.IDPI.CustomPatterns = []string{}

	if err := SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	loaded, _, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}

	if loaded.Server.Bind != "" {
		t.Errorf("loaded bind = %q, want empty string", loaded.Server.Bind)
	}
	if loaded.Security.IDPI.StrictMode {
		t.Errorf("loaded strictMode = %v, want false", loaded.Security.IDPI.StrictMode)
	}
	if len(loaded.Security.IDPI.AllowedDomains) != 0 {
		t.Errorf("loaded allowedDomains = %v, want empty list", loaded.Security.IDPI.AllowedDomains)
	}
	if len(loaded.Browser.ExtensionPaths) != 0 {
		t.Errorf("loaded extensionPaths = %v, want empty list", loaded.Browser.ExtensionPaths)
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input   string
		want    bool
		wantErr bool
	}{
		{"true", true, false},
		{"True", true, false},
		{"TRUE", true, false},
		{"1", true, false},
		{"yes", true, false},
		{"on", true, false},
		{"false", false, false},
		{"False", false, false},
		{"0", false, false},
		{"no", false, false},
		{"off", false, false},
		{"maybe", false, true},
		{"", false, true},
		{"2", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseBool(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBool(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- GetConfigValue ---

func TestGetConfigValue_RoundTrip(t *testing.T) {
	// For every path that SetConfigValue accepts, GetConfigValue must return
	// a string that parses back to the same value.
	triples := []struct {
		path  string
		value string
		want  string // what GetConfigValue should return
	}{
		{"server.port", "8080", "8080"},
		{"server.bind", "0.0.0.0", "0.0.0.0"},
		{"server.token", "s3cr3t", "s3cr3t"},
		{"server.stateDir", "/tmp/state", "/tmp/state"},
		{"browser.version", "120.0", "120.0"},
		{"browser.binary", "/usr/bin/chrome", "/usr/bin/chrome"},
		{"instanceDefaults.mode", "headed", "headed"},
		{"instanceDefaults.noRestore", "true", "true"},
		{"instanceDefaults.blockImages", "false", "false"},
		{"instanceDefaults.blockAds", "1", "true"}, // normalised by parseBool then formatBoolPtr
		{"instanceDefaults.maxTabs", "50", "50"},
		{"instanceDefaults.maxParallelTabs", "8", "8"},
		{"instanceDefaults.userAgent", "MyBot/1.0", "MyBot/1.0"},
		{"instanceDefaults.stealthLevel", "full", "full"},
		{"instanceDefaults.tabEvictionPolicy", "close_lru", "close_lru"},
		{"security.allowEvaluate", "true", "true"},
		{"security.allowMacro", "false", "false"},
		{"security.allowScreencast", "on", "true"},
		{"security.allowDownload", "off", "false"},
		{"security.allowUpload", "yes", "true"},
		{"profiles.baseDir", "/profiles", "/profiles"},
		{"profiles.defaultProfile", "agent", "agent"},
		{"multiInstance.strategy", "explicit", "explicit"},
		{"multiInstance.allocationPolicy", "round_robin", "round_robin"},
		{"multiInstance.instancePortStart", "9900", "9900"},
		{"multiInstance.instancePortEnd", "9950", "9950"},
		{"multiInstance.restart.maxRestarts", "12", "12"},
		{"multiInstance.restart.initBackoffSec", "3", "3"},
		{"multiInstance.restart.maxBackoffSec", "45", "45"},
		{"multiInstance.restart.stableAfterSec", "600", "600"},
		{"security.attach.enabled", "true", "true"},
		{"security.idpi.enabled", "true", "true"},
		{"security.idpi.allowedDomains", "localhost,example.com", "localhost,example.com"},
		{"security.idpi.strictMode", "false", "false"},
		{"security.idpi.scanContent", "true", "true"},
		{"security.idpi.wrapContent", "true", "true"},
		{"security.idpi.customPatterns", "ignore previous instructions,exfiltrate", "ignore previous instructions,exfiltrate"},
		{"timeouts.actionSec", "60", "60"},
		{"timeouts.navigateSec", "90", "90"},
		{"timeouts.shutdownSec", "15", "15"},
		{"timeouts.waitNavMs", "3000", "3000"},
	}

	for _, tt := range triples {
		t.Run(tt.path, func(t *testing.T) {
			fc := &FileConfig{}
			if err := SetConfigValue(fc, tt.path, tt.value); err != nil {
				t.Fatalf("SetConfigValue(%q, %q) error = %v", tt.path, tt.value, err)
			}
			got, err := GetConfigValue(fc, tt.path)
			if err != nil {
				t.Fatalf("GetConfigValue(%q) error = %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("GetConfigValue(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGetConfigValue_NilPointerReturnsEmpty(t *testing.T) {
	fc := &FileConfig{}
	// Pointer fields that have not been set should return "".
	ptrs := []string{
		"instanceDefaults.noRestore",
		"instanceDefaults.blockImages",
		"instanceDefaults.blockMedia",
		"instanceDefaults.blockAds",
		"instanceDefaults.maxTabs",
		"instanceDefaults.maxParallelTabs",
		"instanceDefaults.noAnimations",
		"security.allowEvaluate",
		"security.allowMacro",
		"security.allowScreencast",
		"security.allowDownload",
		"security.allowUpload",
		"multiInstance.instancePortStart",
		"multiInstance.instancePortEnd",
		"security.attach.enabled",
	}
	for _, path := range ptrs {
		t.Run(path, func(t *testing.T) {
			got, err := GetConfigValue(fc, path)
			if err != nil {
				t.Fatalf("GetConfigValue(%q) unexpected error: %v", path, err)
			}
			if got != "" {
				t.Errorf("GetConfigValue(%q) = %q, want empty string for unset pointer", path, got)
			}
		})
	}
}

func TestGetConfigValue_AttachSlices(t *testing.T) {
	fc := &FileConfig{}
	fc.Security.Attach.AllowHosts = []string{"127.0.0.1", "localhost"}
	fc.Security.Attach.AllowSchemes = []string{"ws", "wss"}

	hosts, err := GetConfigValue(fc, "security.attach.allowHosts")
	if err != nil {
		t.Fatalf("GetConfigValue(security.attach.allowHosts) error = %v", err)
	}
	if hosts != "127.0.0.1,localhost" {
		t.Errorf("allowHosts = %q, want %q", hosts, "127.0.0.1,localhost")
	}

	schemes, err := GetConfigValue(fc, "security.attach.allowSchemes")
	if err != nil {
		t.Fatalf("GetConfigValue(security.attach.allowSchemes) error = %v", err)
	}
	if schemes != "ws,wss" {
		t.Errorf("allowSchemes = %q, want %q", schemes, "ws,wss")
	}
}

func TestGetConfigValue_UnknownPaths(t *testing.T) {
	fc := &FileConfig{}
	errorCases := []string{
		"port",                     // missing section
		"",                         // empty
		"unknown.field",            // unknown section
		"server.ghost",             // unknown field in known section
		"security.attach.badfield", // unknown attach field
		"security.idpi.badfield",   // unknown idpi field
	}
	for _, path := range errorCases {
		t.Run(path, func(t *testing.T) {
			_, err := GetConfigValue(fc, path)
			if err == nil {
				t.Errorf("GetConfigValue(%q) should have returned an error", path)
			}
		})
	}
}
