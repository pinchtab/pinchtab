package config

import "testing"

func TestEffectiveFileConfigFillsUnsetKeysFromRuntimeDefaults(t *testing.T) {
	fc := FileConfig{}
	fc.Server.Port = "9999"
	fc.Security.AllowEvaluate = new(true)
	fc.InstanceDefaults.MaxTabs = new(0)
	fc.Security.IDPI = &IDPIConfig{Enabled: true}

	got := EffectiveFileConfig(fc)
	defaults := defaultRuntimeConfig()

	if got.Server.Port != "9999" {
		t.Errorf("server.port = %q, want the file value", got.Server.Port)
	}
	if got.Server.Bind != defaults.Bind {
		t.Errorf("server.bind = %q, want default %q", got.Server.Bind, defaults.Bind)
	}
	if got.Security.AllowEvaluate == nil || !*got.Security.AllowEvaluate {
		t.Error("security.allowEvaluate lost the explicit true")
	}
	if got.Security.AllowMacro == nil || *got.Security.AllowMacro {
		t.Error("security.allowMacro should be filled with the default false")
	}
	if got.InstanceDefaults.MaxTabs == nil || *got.InstanceDefaults.MaxTabs != 0 {
		t.Error("instanceDefaults.maxTabs lost the explicit zero")
	}
	if got.Timeouts.ActionSec != int(defaults.ActionTimeout.Seconds()) {
		t.Errorf("timeouts.actionSec = %d, want default", got.Timeouts.ActionSec)
	}
	if got.Security.IDPI == nil || !got.Security.IDPI.Enabled {
		t.Error("security.idpi lost the explicit enabled")
	}
	if len(got.Security.Attach.AllowSchemes) == 0 {
		t.Error("security.attach.allowSchemes should be filled with the default list")
	}
	if got.Server.StateDir != "" || got.Browser.ExtensionPaths != nil {
		t.Error("host-derived paths must stay as the file has them")
	}
}
