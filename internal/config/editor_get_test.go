package config

import "testing"

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
		{"server.cookieSecure", "false", "false"},
		{"sessions.dashboard.persist", "true", "true"},
		{"sessions.dashboard.maxLifetimeSec", "604800", "604800"},
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
		{"security.allowClipboard", "true", "true"},
		{"security.allowMacro", "false", "false"},
		{"security.allowScreencast", "on", "true"},
		{"security.allowDownload", "off", "false"},
		{"security.downloadAllowedDomains", "pinchtab.com,*.pinchtab.com", "pinchtab.com,*.pinchtab.com"},
		{"security.downloadMaxBytes", "33554432", "33554432"},
		{"security.allowUpload", "yes", "true"},
		{"security.enableActionGuards", "0", "false"},
		{"security.uploadMaxRequestBytes", "12582912", "12582912"},
		{"security.uploadMaxFiles", "12", "12"},
		{"security.uploadMaxFileBytes", "6291456", "6291456"},
		{"security.uploadMaxTotalBytes", "18874368", "18874368"},
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
		"security.allowClipboard",
		"security.allowMacro",
		"security.allowScreencast",
		"security.allowDownload",
		"security.downloadMaxBytes",
		"security.allowUpload",
		"security.enableActionGuards",
		"security.uploadMaxRequestBytes",
		"security.uploadMaxFiles",
		"security.uploadMaxFileBytes",
		"security.uploadMaxTotalBytes",
		"security.maxRedirects",
		"multiInstance.instancePortStart",
		"multiInstance.instancePortEnd",
		"security.attach.enabled",
		"server.cookieSecure",
		"sessions.dashboard.persist",
		"sessions.dashboard.maxLifetimeSec",
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
