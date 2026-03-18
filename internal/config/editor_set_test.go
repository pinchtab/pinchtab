package config

import "testing"

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
		{"security.downloadAllowedDomains", "pinchtab.com, *.pinchtab.com", func(fc *FileConfig) bool {
			return len(fc.Security.DownloadAllowedDomains) == 2 &&
				fc.Security.DownloadAllowedDomains[0] == "pinchtab.com" &&
				fc.Security.DownloadAllowedDomains[1] == "*.pinchtab.com"
		}, false},
		{"security.downloadMaxBytes", "33554432", func(fc *FileConfig) bool {
			return fc.Security.DownloadMaxBytes != nil && *fc.Security.DownloadMaxBytes == 33554432
		}, false},
		{"security.allowUpload", "off", func(fc *FileConfig) bool { return *fc.Security.AllowUpload == false }, false},
		{"security.uploadMaxRequestBytes", "12582912", func(fc *FileConfig) bool {
			return fc.Security.UploadMaxRequestBytes != nil && *fc.Security.UploadMaxRequestBytes == 12582912
		}, false},
		{"security.uploadMaxFiles", "12", func(fc *FileConfig) bool {
			return fc.Security.UploadMaxFiles != nil && *fc.Security.UploadMaxFiles == 12
		}, false},
		{"security.uploadMaxFileBytes", "6291456", func(fc *FileConfig) bool {
			return fc.Security.UploadMaxFileBytes != nil && *fc.Security.UploadMaxFileBytes == 6291456
		}, false},
		{"security.uploadMaxTotalBytes", "18874368", func(fc *FileConfig) bool {
			return fc.Security.UploadMaxTotalBytes != nil && *fc.Security.UploadMaxTotalBytes == 18874368
		}, false},
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
