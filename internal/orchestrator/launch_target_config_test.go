package orchestrator

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// fakeOrch avoids the binary-install side effects of NewOrchestrator.
func fakeOrch(cfg *config.RuntimeConfig) *Orchestrator {
	return &Orchestrator{runtimeCfg: cfg}
}

func runtimeCfgWithTargets() *config.RuntimeConfig {
	return &config.RuntimeConfig{
		Port:             "9867",
		BrowserProvider:  config.BrowserProviderChrome,
		ChromeBinary:     "/global/chrome",
		ChromeExtraFlags: "--global-flag",
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed: "global-seed",
			Platform:        "linux",
		},
		Targets: config.BrowserTargetsConfig{
			"chrome": config.BrowserTargetConfig{
				Provider: config.BrowserProviderChrome,
				Binary:   "/global/chrome",
			},
			"cloak": config.BrowserTargetConfig{
				Provider:   config.BrowserProviderCloak,
				Binary:     "/opt/cloak/cloakbrowser",
				ExtraFlags: "--target-flag",
				Cloak: config.CloakBrowserConfig{
					FingerprintSeed: "target-seed",
					Timezone:        "Europe/London",
				},
			},
		},
		DefaultTarget: "chrome",
	}
}

func TestBuildChildFileConfig_TargetSelectedLaunchPromotesTargetBlock(t *testing.T) {
	cfg := runtimeCfgWithTargets()
	o := fakeOrch(cfg)

	resolved, err := config.ResolveExplicitBrowserTarget(cfg, "cloak")
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	effective := resolved.Config
	fc := o.buildChildFileConfig(effective, "9999", 12345, "/tmp/profile", "/tmp/state", true, nil, nil)

	if fc.Browser.Provider != config.BrowserProviderCloak {
		t.Errorf("Browser.Provider = %q, want cloak", fc.Browser.Provider)
	}
	if fc.Browser.ChromeBinary != "/opt/cloak/cloakbrowser" {
		t.Errorf("Browser.ChromeBinary = %q, want /opt/cloak/cloakbrowser", fc.Browser.ChromeBinary)
	}
	if fc.Browser.ChromeExtraFlags != "--target-flag" {
		t.Errorf("Browser.ChromeExtraFlags = %q, want --target-flag", fc.Browser.ChromeExtraFlags)
	}
	if fc.Browser.Cloak.FingerprintSeed != "target-seed" {
		t.Errorf("Cloak.FingerprintSeed = %q, want target-seed", fc.Browser.Cloak.FingerprintSeed)
	}
	if fc.Browser.Cloak.Timezone != "Europe/London" {
		t.Errorf("Cloak.Timezone = %q, want Europe/London", fc.Browser.Cloak.Timezone)
	}
	if fc.Browser.Cloak.Platform != "linux" {
		t.Errorf("Cloak.Platform = %q, want preserved linux", fc.Browser.Cloak.Platform)
	}
	if cfg.BrowserProvider != config.BrowserProviderChrome ||
		cfg.ChromeBinary != "/global/chrome" {
		t.Errorf("runtime cfg mutated by override: %+v", cfg)
	}
}

func TestBuildChildFileConfig_NoTargetSelected_UsesGlobalFields(t *testing.T) {
	cfg := runtimeCfgWithTargets()
	o := fakeOrch(cfg)

	fc := o.buildChildFileConfig(cfg, "9999", 12345, "/tmp/profile", "/tmp/state", true, nil, nil)
	if fc.Browser.Provider != config.BrowserProviderChrome {
		t.Errorf("Browser.Provider = %q, want chrome (global)", fc.Browser.Provider)
	}
	if fc.Browser.ChromeBinary != "/global/chrome" {
		t.Errorf("Browser.ChromeBinary = %q, want /global/chrome", fc.Browser.ChromeBinary)
	}
	if fc.Browser.ChromeExtraFlags != "--global-flag" {
		t.Errorf("Browser.ChromeExtraFlags = %q, want --global-flag", fc.Browser.ChromeExtraFlags)
	}
}

func TestBuildChildFileConfig_LegacyConfigByteStable(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Port:             "9867",
		BrowserProvider:  config.BrowserProviderChrome,
		ChromeBinary:     "/usr/bin/chrome",
		ChromeExtraFlags: "--flag",
	}
	o := fakeOrch(cfg)

	prev := o.buildChildFileConfig(o.runtimeCfg, "9999", 12345, "/tmp/profile", "/tmp/state", true, nil, nil)
	prevBytes, err := json.MarshalIndent(prev, "", "  ")
	if err != nil {
		t.Fatalf("marshal prev: %v", err)
	}

	got := o.buildChildFileConfig(cfg, "9999", 12345, "/tmp/profile", "/tmp/state", true, nil, nil)
	gotBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}

	if string(prevBytes) != string(gotBytes) {
		t.Fatalf("legacy config child bytes differ.\nprev:\n%s\ngot:\n%s", prevBytes, gotBytes)
	}
}

func TestWriteAttachChildConfigPreservesRuntimeModeAndEngine(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Port:                   "9867",
		StateDir:               t.TempDir(),
		BrowserProvider:        config.BrowserProviderChrome,
		Headless:               true,
		HeadlessSet:            true,
		Engine:                 "lite",
		AttachEnabled:          true,
		AttachAllowHosts:       []string{"*"},
		AttachAllowSchemes:     []string{"http", "ws"},
		AttachForwardProxyAuth: true,
	}
	o := fakeOrch(cfg)

	stateDir := t.TempDir()
	configPath, err := o.writeAttachChildConfig("9999", config.BrowserProviderCloak, stateDir)
	if err != nil {
		t.Fatalf("writeAttachChildConfig: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read attach child config: %v", err)
	}
	var fc config.FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("unmarshal attach child config: %v", err)
	}

	if fc.Server.Engine != "lite" {
		t.Errorf("Server.Engine = %q, want lite", fc.Server.Engine)
	}
	if fc.InstanceDefaults.Mode != "headless" {
		t.Errorf("InstanceDefaults.Mode = %q, want headless", fc.InstanceDefaults.Mode)
	}
	if fc.Browser.Provider != config.BrowserProviderCloak {
		t.Errorf("Browser.Provider = %q, want cloak", fc.Browser.Provider)
	}
	if fc.Security.Attach.Enabled == nil || *fc.Security.Attach.Enabled {
		t.Errorf("Security.Attach.Enabled = %v, want false", fc.Security.Attach.Enabled)
	}
	if len(fc.Security.Attach.AllowHosts) != 0 {
		t.Errorf("Security.Attach.AllowHosts = %v, want empty", fc.Security.Attach.AllowHosts)
	}
	if len(fc.Security.Attach.AllowSchemes) != 0 {
		t.Errorf("Security.Attach.AllowSchemes = %v, want empty", fc.Security.Attach.AllowSchemes)
	}
	if fc.Security.Attach.ForwardProxyAuth == nil || *fc.Security.Attach.ForwardProxyAuth {
		t.Errorf("Security.Attach.ForwardProxyAuth = %v, want false", fc.Security.Attach.ForwardProxyAuth)
	}
}
