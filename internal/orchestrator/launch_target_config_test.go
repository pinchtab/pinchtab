package orchestrator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// fakeOrch avoids the binary-install side effects of NewOrchestrator.
func fakeOrch(cfg *config.RuntimeConfig) *Orchestrator {
	return &Orchestrator{runtimeCfg: cfg}
}

func runtimeCfgWithTargets() *config.RuntimeConfig {
	return &config.RuntimeConfig{
		Port:              "9867",
		DefaultBrowser:    config.BrowserChrome,
		BrowserBinary:     "/global/chrome",
		BrowserExtraFlags: "--global-flag",
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed: "global-seed",
			Platform:        "linux",
		},
		Targets: config.BrowserTargetsConfig{
			"chrome": config.BrowserTargetConfig{
				Provider: config.BrowserChrome,
				Binary:   "/global/chrome",
			},
			"cloak": config.BrowserTargetConfig{
				Provider:   config.BrowserCloak,
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

	if fc.Browsers.Default != config.BrowserCloak {
		t.Errorf("Browsers.Default = %q, want cloak", fc.Browsers.Default)
	}
	if fc.Browser.BrowserBinary != "/opt/cloak/cloakbrowser" {
		t.Errorf("Browser.BrowserBinary = %q, want /opt/cloak/cloakbrowser", fc.Browser.BrowserBinary)
	}
	if fc.Browser.BrowserExtraFlags != "--target-flag" {
		t.Errorf("Browser.BrowserExtraFlags = %q, want --target-flag", fc.Browser.BrowserExtraFlags)
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
	if cfg.DefaultBrowser != config.BrowserChrome ||
		cfg.BrowserBinary != "/global/chrome" {
		t.Errorf("runtime cfg mutated by override: %+v", cfg)
	}
}

func TestBuildChildFileConfig_NoTargetSelected_UsesGlobalFields(t *testing.T) {
	cfg := runtimeCfgWithTargets()
	o := fakeOrch(cfg)

	fc := o.buildChildFileConfig(cfg, "9999", 12345, "/tmp/profile", "/tmp/state", true, nil, nil)
	if fc.Browsers.Default != config.BrowserChrome {
		t.Errorf("Browsers.Default = %q, want chrome (global)", fc.Browsers.Default)
	}
	if fc.Browser.BrowserBinary != "/global/chrome" {
		t.Errorf("Browser.BrowserBinary = %q, want /global/chrome", fc.Browser.BrowserBinary)
	}
	if fc.Browser.BrowserExtraFlags != "--global-flag" {
		t.Errorf("Browser.BrowserExtraFlags = %q, want --global-flag", fc.Browser.BrowserExtraFlags)
	}
}

func TestBuildChildFileConfig_LegacyConfigByteStable(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Port:              "9867",
		DefaultBrowser:    config.BrowserChrome,
		BrowserBinary:     "/usr/bin/chrome",
		BrowserExtraFlags: "--flag",
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
		DefaultBrowser:         config.BrowserChrome,
		Headless:               true,
		HeadlessSet:            true,
		AttachEnabled:          true,
		AttachAllowHosts:       []string{"*"},
		AttachAllowSchemes:     []string{"http", "ws"},
		AttachForwardProxyAuth: true,
	}
	o := fakeOrch(cfg)

	stateDir := t.TempDir()
	configPath, err := o.writeAttachChildConfig("9999", config.BrowserCloak, stateDir)
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

	if fc.InstanceDefaults.Mode != "headless" {
		t.Errorf("InstanceDefaults.Mode = %q, want headless", fc.InstanceDefaults.Mode)
	}
	if fc.Browsers.Default != config.BrowserCloak {
		t.Errorf("Browsers.Default = %q, want cloak", fc.Browsers.Default)
	}
	if fc.Security.Attach.Enabled == nil || *fc.Security.Attach.Enabled {
		t.Errorf("Security.Attach.Enabled = %v, want false", fc.Security.Attach.Enabled)
	}
	if got := strings.Join(fc.Security.Attach.AllowHosts, ","); got != "*" {
		t.Errorf("Security.Attach.AllowHosts = %q, want *", got)
	}
	if got := strings.Join(fc.Security.Attach.AllowSchemes, ","); got != "http,ws" {
		t.Errorf("Security.Attach.AllowSchemes = %q, want http,ws", got)
	}
	if fc.Security.Attach.ForwardProxyAuth == nil || *fc.Security.Attach.ForwardProxyAuth {
		t.Errorf("Security.Attach.ForwardProxyAuth = %v, want false", fc.Security.Attach.ForwardProxyAuth)
	}
}
