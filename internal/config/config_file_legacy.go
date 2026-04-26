package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// legacyFileConfig is the old flat structure for backward compatibility.
type legacyFileConfig struct {
	Port              string `json:"port"`
	InstancePortStart *int   `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int   `json:"instancePortEnd,omitempty"`
	Token             string `json:"token,omitempty"`
	AllowEvaluate     *bool  `json:"allowEvaluate,omitempty"`
	AllowMacro        *bool  `json:"allowMacro,omitempty"`
	AllowScreencast   *bool  `json:"allowScreencast,omitempty"`
	AllowDownload     *bool  `json:"allowDownload,omitempty"`
	AllowUpload       *bool  `json:"allowUpload,omitempty"`
	AllowClipboard    *bool  `json:"allowClipboard,omitempty"`
	StateDir          string `json:"stateDir"`
	ProfileDir        string `json:"profileDir"`
	Headless          *bool  `json:"headless,omitempty"`
	NoRestore         bool   `json:"noRestore"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	TimeoutSec        int    `json:"timeoutSec,omitempty"`
	NavigateSec       int    `json:"navigateSec,omitempty"`
}

// convertLegacyConfig converts flat config to nested structure.
func convertLegacyConfig(lc *legacyFileConfig) *FileConfig {
	fc := &FileConfig{}

	fc.Server.Port = lc.Port
	fc.Server.Token = lc.Token
	fc.Server.StateDir = lc.StateDir

	if lc.Headless != nil {
		if *lc.Headless {
			fc.InstanceDefaults.Mode = "headless"
		} else {
			fc.InstanceDefaults.Mode = "headed"
		}
	}
	fc.InstanceDefaults.MaxTabs = lc.MaxTabs
	if lc.NoRestore {
		b := true
		fc.InstanceDefaults.NoRestore = &b
	}

	if lc.ProfileDir != "" {
		fc.Profiles.BaseDir = filepath.Dir(lc.ProfileDir)
		fc.Profiles.DefaultProfile = filepath.Base(lc.ProfileDir)
	}

	fc.Security.AllowEvaluate = lc.AllowEvaluate
	fc.Security.AllowMacro = lc.AllowMacro
	fc.Security.AllowScreencast = lc.AllowScreencast
	fc.Security.AllowDownload = lc.AllowDownload
	fc.Security.AllowUpload = lc.AllowUpload
	fc.Security.AllowClipboard = lc.AllowClipboard

	fc.Timeouts.ActionSec = lc.TimeoutSec
	fc.Timeouts.NavigateSec = lc.NavigateSec

	fc.MultiInstance.InstancePortStart = lc.InstancePortStart
	fc.MultiInstance.InstancePortEnd = lc.InstancePortEnd

	return fc
}

// isLegacyConfig detects if JSON is flat (legacy) or nested (new).
func isLegacyConfig(data []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}

	newKeys := []string{"server", "browser", "instanceDefaults", "profiles", "multiInstance", "security", "attach", "timeouts", "sessions"}
	for _, key := range newKeys {
		if _, has := probe[key]; has {
			return false
		}
	}

	if _, hasPort := probe["port"]; hasPort {
		return true
	}
	if _, hasHeadless := probe["headless"]; hasHeadless {
		return true
	}

	return false
}

func modeToHeadless(mode string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return fallback
	case "headless":
		return true
	case "headed":
		return false
	default:
		return fallback
	}
}
