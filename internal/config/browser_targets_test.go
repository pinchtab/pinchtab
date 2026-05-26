package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
)

func TestMigrateLegacyBrowserConfig_LegacyOnlySynthesizesDefaultTarget(t *testing.T) {
	bc := BrowserConfig{
		Provider:     BrowserChrome,
		ChromeBinary: "/usr/bin/chrome",
	}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc)
	if !synthesized {
		t.Fatalf("expected synthesized=true, got false")
	}
	if conflict {
		t.Fatalf("expected conflict=false, got true")
	}
	if bc.DefaultTarget != DefaultBrowserTargetName {
		t.Fatalf("DefaultTarget = %q, want %q", bc.DefaultTarget, DefaultBrowserTargetName)
	}
	tgt, ok := bc.Targets[DefaultBrowserTargetName]
	if !ok {
		t.Fatalf("targets[%q] missing", DefaultBrowserTargetName)
	}
	if tgt.Provider != BrowserChrome {
		t.Fatalf("target.Provider = %q, want chrome", tgt.Provider)
	}
	if tgt.Binary != "/usr/bin/chrome" {
		t.Fatalf("target.Binary = %q, want /usr/bin/chrome", tgt.Binary)
	}
}

func TestMigrateLegacyBrowserConfig_CloakLegacyCopiesCloakSubBlock(t *testing.T) {
	bc := BrowserConfig{
		Provider:     BrowserCloak,
		ChromeBinary: "/opt/cloak/chrome",
		Cloak: CloakBrowserConfig{
			FingerprintSeed: "42",
			Platform:        "linux",
		},
	}
	synthesized, _ := migrateLegacyBrowserConfig(&bc)
	if !synthesized {
		t.Fatalf("expected synthesized=true")
	}
	tgt := bc.Targets[DefaultBrowserTargetName]
	if tgt.Cloak.FingerprintSeed != "42" || tgt.Cloak.Platform != "linux" {
		t.Fatalf("cloak sub-block not copied; got %+v", tgt.Cloak)
	}
}

func TestMigrateLegacyBrowserConfig_DeepClonesLegacyTargetFields(t *testing.T) {
	quota := 128
	disableStealth := true
	bc := BrowserConfig{
		Provider: BrowserChrome,
		Cloak: CloakBrowserConfig{
			StorageQuotaMB:            &quota,
			DisableDefaultStealthArgs: &disableStealth,
		},
		Proxy: BrowserProxyConfig{
			Server:     "http://proxy.example:8080",
			BypassList: []string{"localhost"},
			Geo:        &BrowserProxyGeoConfig{Timezone: "UTC"},
		},
	}
	synthesized, _ := migrateLegacyBrowserConfig(&bc)
	if !synthesized {
		t.Fatalf("expected synthesized=true")
	}

	quota = 256
	disableStealth = false
	bc.Proxy.BypassList[0] = "mutated"
	bc.Proxy.Geo.Timezone = "Europe/London"

	tgt := bc.Targets[DefaultBrowserTargetName]
	if tgt.Cloak.StorageQuotaMB == nil || *tgt.Cloak.StorageQuotaMB != 128 {
		t.Fatalf("target quota shared with legacy config: %+v", tgt.Cloak.StorageQuotaMB)
	}
	if tgt.Cloak.DisableDefaultStealthArgs == nil || !*tgt.Cloak.DisableDefaultStealthArgs {
		t.Fatalf("target stealth pointer shared with legacy config: %+v", tgt.Cloak.DisableDefaultStealthArgs)
	}
	if got := tgt.Proxy.BypassList[0]; got != "localhost" {
		t.Fatalf("target proxy bypass shared with legacy config: %q", got)
	}
	if got := tgt.Proxy.Geo.Timezone; got != "UTC" {
		t.Fatalf("target proxy geo shared with legacy config: %q", got)
	}
}

func TestMigrateLegacyBrowserConfig_TargetsOnlyLeftIntact(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "chrome-local",
		Targets: BrowserTargetsConfig{
			"chrome-local": {Provider: BrowserChrome, Binary: "/usr/bin/chrome"},
		},
	}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc)
	if synthesized {
		t.Fatalf("expected synthesized=false when targets are already present")
	}
	if conflict {
		t.Fatalf("expected conflict=false when no legacy fields are set")
	}
	if bc.DefaultTarget != "chrome-local" {
		t.Fatalf("DefaultTarget mutated: %q", bc.DefaultTarget)
	}
	if len(bc.Targets) != 1 {
		t.Fatalf("Targets mutated: %+v", bc.Targets)
	}
}

func TestMigrateLegacyBrowserConfig_BothPresentExplicitWinsAndFlagsConflict(t *testing.T) {
	bc := BrowserConfig{
		Provider:      BrowserChrome,
		ChromeBinary:  "/legacy/chrome",
		DefaultTarget: "explicit",
		Targets: BrowserTargetsConfig{
			"explicit": {Provider: BrowserChrome, Binary: "/explicit/chrome"},
		},
	}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc)
	if synthesized {
		t.Fatalf("explicit targets must NOT be overwritten (synthesized should be false)")
	}
	if !conflict {
		t.Fatalf("expected conflict=true when both blocks are set")
	}
	if _, ok := bc.Targets[DefaultBrowserTargetName]; ok {
		t.Fatalf("must not synthesize 'default' when explicit targets exist")
	}
	if bc.DefaultTarget != "explicit" {
		t.Fatalf("explicit defaultTarget overwritten: %q", bc.DefaultTarget)
	}
	// Legacy fields preserved on the file config for round-trip.
	if bc.Provider != BrowserChrome || bc.ChromeBinary != "/legacy/chrome" {
		t.Fatalf("legacy fields lost; got Provider=%q Binary=%q", bc.Provider, bc.ChromeBinary)
	}
}

func TestMigrateLegacyBrowserConfig_EmptyConfigNoop(t *testing.T) {
	bc := BrowserConfig{}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc)
	if synthesized || conflict {
		t.Fatalf("empty browser config should be a no-op; got synth=%v conflict=%v", synthesized, conflict)
	}
	if len(bc.Targets) != 0 || bc.DefaultTarget != "" {
		t.Fatalf("empty config mutated: %+v", bc)
	}
}

func TestValidateBrowserTargets_NoTargets_Nil(t *testing.T) {
	if errs := ValidateBrowserTargets(BrowserConfig{}); errs != nil {
		t.Fatalf("expected nil errors for empty targets; got %v", errs)
	}
}

func TestValidateBrowserTargets_GoodConfig_NoErrors(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "chrome-local",
		FallbackOrder: []string{"cloak-primary"},
		Targets: BrowserTargetsConfig{
			"chrome-local":  {Provider: BrowserChrome, Binary: "/usr/bin/chrome"},
			"cloak-primary": {Provider: BrowserCloak, Binary: "/opt/cloak/chrome"},
		},
	}
	if errs := ValidateBrowserTargets(bc); len(errs) != 0 {
		t.Fatalf("expected no errors; got %v", errs)
	}
}

func TestValidateBrowserTargets_RejectsBadTargetName(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "Bad_Name",
		Targets: BrowserTargetsConfig{
			"Bad_Name": {Provider: BrowserChrome},
		},
	}
	errs := ValidateBrowserTargets(bc)
	if len(errs) == 0 {
		t.Fatalf("expected error for bad target name")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "invalid target name") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing invalid-name error; got %v", errs)
	}
}

func TestValidateBrowserTargets_RejectsUnknownProvider(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "ok",
		Targets: BrowserTargetsConfig{
			"ok": {Provider: "lightpanda"},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "unknown provider") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown-provider error; got %v", errs)
	}
}

func TestValidateBrowserTargets_RejectsMissingProvider(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "ok",
		Targets: BrowserTargetsConfig{
			"ok": {Binary: "/foo"},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "provider is required") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected required-provider error; got %v", errs)
	}
}

func TestValidateBrowserTargets_RejectsCloakTargetWithoutBinary(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "cloak",
		Targets: BrowserTargetsConfig{
			"cloak": {Provider: BrowserCloak},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browser.targets.cloak.binary") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cloak target binary error; got %v", errs)
	}
}

func TestValidateBrowserTargets_CloakTargetCanInheritGlobalBinary(t *testing.T) {
	bc := BrowserConfig{
		ChromeBinary:     "/opt/cloak/chrome",
		DefaultTarget:    "cloak",
		ChromeExtraFlags: "--global-flag",
		Targets: BrowserTargetsConfig{
			"cloak": {Provider: BrowserCloak},
		},
	}
	if errs := ValidateBrowserTargets(bc); len(errs) != 0 {
		t.Fatalf("expected no errors; got %v", errs)
	}
}

func TestValidateBrowserTargets_DanglingFallbackReference(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "a",
		FallbackOrder: []string{"missing"},
		Targets: BrowserTargetsConfig{
			"a": {Provider: BrowserChrome},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "fallbackOrder") && strings.Contains(e.Error(), "missing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected dangling-fallback error; got %v", errs)
	}
}

func TestValidateBrowserTargets_RejectsFallbackDuplicateAndDefaultSelfReference(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "a",
		FallbackOrder: []string{"b", "a", "b"},
		Targets: BrowserTargetsConfig{
			"a": {Provider: BrowserChrome},
			"b": {Provider: BrowserChrome},
		},
	}
	errs := ValidateBrowserTargets(bc)
	var sawSelf, sawDuplicate bool
	for _, e := range errs {
		msg := e.Error()
		if strings.Contains(msg, "must not include defaultTarget") {
			sawSelf = true
		}
		if strings.Contains(msg, "duplicates target") {
			sawDuplicate = true
		}
	}
	if !sawSelf || !sawDuplicate {
		t.Fatalf("expected self-reference and duplicate errors; got %v", errs)
	}
}

func TestValidateBrowserTargets_RejectsUnsafeTargetExtraFlags(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "a",
		Targets: BrowserTargetsConfig{
			"a": {Provider: BrowserChrome, ExtraFlags: "--disable-web-security"},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browser.targets.a.extraFlags") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected target extraFlags validation error; got %v", errs)
	}
}

func TestValidateBrowserTargets_ValidatesTargetCloakConfig(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "cloak",
		Targets: BrowserTargetsConfig{
			"cloak": {
				Provider: BrowserCloak,
				Binary:   "/opt/cloak/chrome",
				Cloak:    CloakBrowserConfig{Locale: "en-gb"},
			},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browser.targets.cloak.cloak") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected target cloak validation error; got %v", errs)
	}
}

func TestValidateBrowserTargets_DefaultTargetMissingWithMultipleTargets(t *testing.T) {
	bc := BrowserConfig{
		Targets: BrowserTargetsConfig{
			"a": {Provider: BrowserChrome},
			"b": {Provider: BrowserChrome},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browser.defaultTarget") && strings.Contains(e.Error(), "multiple") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing-defaultTarget error; got %v", errs)
	}
}

func TestValidateBrowserTargets_DefaultTargetReferencingUnknown(t *testing.T) {
	bc := BrowserConfig{
		DefaultTarget: "ghost",
		Targets: BrowserTargetsConfig{
			"a": {Provider: BrowserChrome},
		},
	}
	errs := ValidateBrowserTargets(bc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browser.defaultTarget") && strings.Contains(e.Error(), "ghost") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown defaultTarget error; got %v", errs)
	}
}

func TestValidateBrowserTargets_SingleTargetNoDefaultIsAllowed(t *testing.T) {
	bc := BrowserConfig{
		Targets: BrowserTargetsConfig{
			"only": {Provider: BrowserChrome},
		},
	}
	if errs := ValidateBrowserTargets(bc); len(errs) != 0 {
		t.Fatalf("single target without defaultTarget should validate; got %v", errs)
	}
}

func TestResolveDefaultTarget_ReturnsExplicit(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "primary",
		Targets: BrowserTargetsConfig{
			"primary": {Provider: BrowserChrome},
			"backup":  {Provider: BrowserChrome},
		},
	}
	if got := ResolveDefaultTarget(cfg); got != "primary" {
		t.Fatalf("ResolveDefaultTarget = %q, want primary", got)
	}
}

func TestResolveDefaultTarget_SingleTargetAutoPicks(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"only": {Provider: BrowserChrome},
		},
	}
	if got := ResolveDefaultTarget(cfg); got != "only" {
		t.Fatalf("ResolveDefaultTarget = %q, want only", got)
	}
}

func TestResolveDefaultTarget_NoTargetsReturnsEmpty(t *testing.T) {
	cfg := &RuntimeConfig{}
	if got := ResolveDefaultTarget(cfg); got != "" {
		t.Fatalf("ResolveDefaultTarget = %q, want empty", got)
	}
}

func TestResolveDefaultBrowserTarget_LegacyNoTargetsReturnsFreshConfig(t *testing.T) {
	cookieSecure := true
	cfg := &RuntimeConfig{
		DefaultBrowser: BrowserCloak,
		CookieSecure:   &cookieSecure,
		AllowedDomains: []string{"example.com"},
	}
	resolved, err := ResolveDefaultBrowserTarget(cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resolved == nil || !resolved.Legacy {
		t.Fatalf("resolved legacy = %+v, want legacy result", resolved)
	}
	if resolved.Name != "" || resolved.Provider != BrowserCloak {
		t.Fatalf("resolved = %+v, want unnamed cloak legacy target", resolved)
	}
	if resolved.Config == nil || resolved.Config == cfg {
		t.Fatalf("resolved.Config should be a fresh clone, got %+v", resolved.Config)
	}

	resolved.Config.AllowedDomains[0] = "mutated.example"
	*resolved.Config.CookieSecure = false
	if cfg.AllowedDomains[0] != "example.com" {
		t.Fatalf("legacy resolved config shared AllowedDomains with input: %+v", cfg.AllowedDomains)
	}
	if cfg.CookieSecure == nil || !*cfg.CookieSecure {
		t.Fatalf("legacy resolved config shared CookieSecure pointer with input: %+v", cfg.CookieSecure)
	}
}

func TestResolveDefaultBrowserTarget_SingleTargetPromotesEffectiveConfig(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultBrowser:   BrowserChrome,
		ChromeBinary:     "/global/chrome",
		ChromeExtraFlags: "--global-flag",
		Targets: BrowserTargetsConfig{
			"only": {
				Provider:   BrowserCloak,
				Binary:     "/opt/cloak/chrome",
				ExtraFlags: "--target-flag",
				Cloak:      CloakBrowserConfig{Timezone: "UTC"},
			},
		},
	}
	resolved, err := ResolveDefaultBrowserTarget(cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resolved.Name != "only" || resolved.Provider != BrowserCloak || resolved.Legacy {
		t.Fatalf("resolved = %+v, want non-legacy cloak target named only", resolved)
	}
	if resolved.Config == cfg {
		t.Fatalf("resolved.Config should not reuse input pointer")
	}
	if resolved.Config.DefaultBrowser != BrowserCloak {
		t.Fatalf("effective provider = %q, want cloak", resolved.Config.DefaultBrowser)
	}
	if resolved.Config.ChromeBinary != "/opt/cloak/chrome" {
		t.Fatalf("effective binary = %q, want /opt/cloak/chrome", resolved.Config.ChromeBinary)
	}
	if resolved.Config.ChromeExtraFlags != "--target-flag" {
		t.Fatalf("effective extraFlags = %q, want --target-flag", resolved.Config.ChromeExtraFlags)
	}
	if resolved.Config.Cloak.Timezone != "UTC" {
		t.Fatalf("effective cloak timezone = %q, want UTC", resolved.Config.Cloak.Timezone)
	}
	if cfg.DefaultBrowser != BrowserChrome || cfg.ChromeBinary != "/global/chrome" {
		t.Fatalf("input cfg was mutated: %+v", cfg)
	}
}

func TestResolveExplicitBrowserTarget_RejectsEmptyName(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"only": {Provider: BrowserChrome},
		},
	}
	if _, err := ResolveExplicitBrowserTarget(cfg, " "); err == nil {
		t.Fatal("expected error for empty explicit browser target")
	}
}

func TestResolveExplicitBrowserTarget_ConfigDoesNotAliasOriginal(t *testing.T) {
	cookieSecure := true
	cfg := &RuntimeConfig{
		DefaultBrowser:         BrowserChrome,
		CookieSecure:           &cookieSecure,
		AllowedDomains:         []string{"allowed.example"},
		DownloadAllowedDomains: []string{"download.example"},
		TrustedProxyCIDRs:      []string{"10.0.0.0/24"},
		TrustedResolveCIDRs:    []string{"192.168.0.0/24"},
		ExtensionPaths:         []string{"/ext/global"},
		FallbackOrder:          []string{"backup"},
		AttachAllowHosts:       []string{"localhost"},
		AttachAllowSchemes:     []string{"ws"},
		IDPI:                   IDPIConfig{CustomPatterns: []string{"ignore-me"}},
		AutoSolver:             AutoSolverConfig{Solvers: []string{"semantic"}},
		Proxy:                  BrowserProxyConfig{Server: "http://global.example:8080", BypassList: []string{"global"}, Geo: &BrowserProxyGeoConfig{Timezone: "UTC"}},
		Cloak:                  CloakBrowserRuntimeConfig{FingerprintSeed: "global"},
		DefaultTarget:          "proxy",
		Targets: BrowserTargetsConfig{
			"proxy": {
				Provider: BrowserChrome,
				Proxy: BrowserProxyConfig{
					Server:     "http://proxy.example:8080",
					BypassList: []string{"target"},
					Geo:        &BrowserProxyGeoConfig{Timezone: "UTC"},
				},
				Cloak: CloakBrowserConfig{
					FingerprintSeed: "target",
					StorageQuotaMB:  intPtr(256),
				},
			},
			"backup": {Provider: BrowserChrome},
		},
	}

	resolved, err := ResolveExplicitBrowserTarget(cfg, "proxy")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := resolved.Config
	if got == cfg {
		t.Fatal("resolved config reused input pointer")
	}
	if got.Targets == nil || len(got.Targets) != len(cfg.Targets) {
		t.Fatalf("resolved targets = %+v, want cloned targets map", got.Targets)
	}

	*got.CookieSecure = false
	got.AllowedDomains[0] = "mutated"
	got.DownloadAllowedDomains[0] = "mutated"
	got.TrustedProxyCIDRs[0] = "mutated"
	got.TrustedResolveCIDRs[0] = "mutated"
	got.ExtensionPaths[0] = "mutated"
	got.FallbackOrder[0] = "mutated"
	got.AttachAllowHosts[0] = "mutated"
	got.AttachAllowSchemes[0] = "mutated"
	got.IDPI.CustomPatterns[0] = "mutated"
	got.AutoSolver.Solvers[0] = "mutated"
	got.Proxy.BypassList[0] = "mutated"
	got.Proxy.Geo.Timezone = "Europe/London"
	resolved.Target.Proxy.BypassList[0] = "mutated-resolved-target"
	resolved.Target.Proxy.Geo.Timezone = "America/New_York"
	target := got.Targets["proxy"]
	target.Proxy.BypassList[0] = "mutated-target-map"
	target.Proxy.Geo.Timezone = "Asia/Tokyo"
	target.Cloak.StorageQuotaMB = intPtr(512)
	got.Targets["proxy"] = target

	if cfg.CookieSecure == nil || !*cfg.CookieSecure {
		t.Fatalf("CookieSecure pointer shared with resolved config")
	}
	if cfg.AllowedDomains[0] != "allowed.example" ||
		cfg.DownloadAllowedDomains[0] != "download.example" ||
		cfg.TrustedProxyCIDRs[0] != "10.0.0.0/24" ||
		cfg.TrustedResolveCIDRs[0] != "192.168.0.0/24" ||
		cfg.ExtensionPaths[0] != "/ext/global" ||
		cfg.FallbackOrder[0] != "backup" ||
		cfg.AttachAllowHosts[0] != "localhost" ||
		cfg.AttachAllowSchemes[0] != "ws" ||
		cfg.IDPI.CustomPatterns[0] != "ignore-me" ||
		cfg.AutoSolver.Solvers[0] != "semantic" {
		t.Fatalf("resolved config shared a top-level slice with input: %+v", cfg)
	}
	if cfg.Proxy.BypassList[0] != "global" || cfg.Proxy.Geo.Timezone != "UTC" {
		t.Fatalf("global proxy shared with resolved config: %+v", cfg.Proxy)
	}
	originalTarget := cfg.Targets["proxy"]
	if originalTarget.Proxy.BypassList[0] != "target" || originalTarget.Proxy.Geo.Timezone != "UTC" {
		t.Fatalf("target proxy shared with resolved config: %+v", originalTarget.Proxy)
	}
	if originalTarget.Cloak.StorageQuotaMB == nil || *originalTarget.Cloak.StorageQuotaMB != 256 {
		t.Fatalf("target cloak pointer shared with resolved config: %+v", originalTarget.Cloak.StorageQuotaMB)
	}
}

func TestResolveRequestedTarget_LegacyEmptyRequested(t *testing.T) {
	cfg := &RuntimeConfig{}
	name, provider, err := ResolveRequestedTarget(cfg, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "" || provider != "" {
		t.Fatalf("got (%q, %q), want empty/empty", name, provider)
	}
}

func TestResolveRequestedTarget_LegacyWithRequestedErrors(t *testing.T) {
	cfg := &RuntimeConfig{}
	_, _, err := ResolveRequestedTarget(cfg, "chrome")
	if err == nil {
		t.Fatal("expected error when requesting target with no targets configured")
	}
	if !strings.Contains(err.Error(), "no browser targets configured") {
		t.Fatalf("err = %v, want 'no browser targets configured'", err)
	}
}

func TestResolveRequestedTarget_TargetsEmptyRequestedReturnsDefault(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "primary",
		Targets: BrowserTargetsConfig{
			"primary": {Provider: BrowserChrome},
			"backup":  {Provider: BrowserCloak},
		},
	}
	name, provider, err := ResolveRequestedTarget(cfg, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "primary" || provider != BrowserChrome {
		t.Fatalf("got (%q, %q), want (primary, chrome)", name, provider)
	}
}

func TestResolveRequestedTarget_TargetsMultiNoDefaultErrors(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"a": {Provider: BrowserChrome},
			"b": {Provider: BrowserChrome},
		},
	}
	_, _, err := ResolveRequestedTarget(cfg, "")
	if err == nil {
		t.Fatal("expected error when no default and multiple targets")
	}
	if !strings.Contains(err.Error(), "no default browser target") {
		t.Fatalf("err = %v, want 'no default browser target'", err)
	}
}

func TestResolveRequestedTarget_ValidRequestedReturnsProvider(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "primary",
		Targets: BrowserTargetsConfig{
			"primary": {Provider: BrowserChrome},
			"cloak-1": {Provider: BrowserCloak},
		},
	}
	name, provider, err := ResolveRequestedTarget(cfg, "cloak-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "cloak-1" || provider != BrowserCloak {
		t.Fatalf("got (%q, %q), want (cloak-1, cloak)", name, provider)
	}
}

func TestResolveRequestedTarget_UnknownRequestedErrors(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "primary",
		Targets: BrowserTargetsConfig{
			"primary": {Provider: BrowserChrome},
		},
	}
	_, _, err := ResolveRequestedTarget(cfg, "ghost")
	if err == nil {
		t.Fatal("expected error for unknown requested target")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want 'not found'", err)
	}
}

func TestResolveRequestedTarget_InvalidNameErrors(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "primary",
		Targets: BrowserTargetsConfig{
			"primary": {Provider: BrowserChrome},
		},
	}
	_, _, err := ResolveRequestedTarget(cfg, "Bad_Name")
	if err == nil {
		t.Fatal("expected error for invalid target name")
	}
	if !strings.Contains(err.Error(), "invalid browser target name") {
		t.Fatalf("err = %v, want 'invalid browser target name'", err)
	}
}

// --- TargetsForBrowser ---

func TestTargetsForBrowser_SingleMatch(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"cloak-default": {Provider: BrowserCloak},
			"chrome-local":  {Provider: BrowserChrome},
		},
	}
	got := TargetsForBrowser(cfg, "cloak")
	if len(got) != 1 || got[0] != "cloak-default" {
		t.Fatalf("TargetsForBrowser(cloak) = %v, want [cloak-default]", got)
	}
}

func TestTargetsForBrowser_MultipleMatches(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"cloak-eu": {Provider: BrowserCloak},
			"cloak-us": {Provider: BrowserCloak},
			"chrome":   {Provider: BrowserChrome},
		},
	}
	got := TargetsForBrowser(cfg, "cloak")
	if len(got) != 2 || got[0] != "cloak-eu" || got[1] != "cloak-us" {
		t.Fatalf("TargetsForBrowser(cloak) = %v, want [cloak-eu cloak-us]", got)
	}
}

func TestTargetsForBrowser_NoMatch(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"chrome-local": {Provider: BrowserChrome},
		},
	}
	got := TargetsForBrowser(cfg, "cloak")
	if len(got) != 0 {
		t.Fatalf("TargetsForBrowser(cloak) = %v, want empty", got)
	}
}

func TestTargetsForBrowser_NilConfig(t *testing.T) {
	got := TargetsForBrowser(nil, "chrome")
	if got != nil {
		t.Fatalf("TargetsForBrowser(nil) = %v, want nil", got)
	}
}

func TestTargetsForBrowser_EmptyTargets(t *testing.T) {
	cfg := &RuntimeConfig{}
	got := TargetsForBrowser(cfg, "chrome")
	if got != nil {
		t.Fatalf("TargetsForBrowser(empty) = %v, want nil", got)
	}
}

// --- ResolveBrowserToTarget ---

func TestResolveBrowserToTarget_SingleTarget(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"cloak-default": {Provider: BrowserCloak},
			"chrome-local":  {Provider: BrowserChrome},
		},
	}
	got, err := ResolveBrowserToTarget(cfg, "cloak")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "cloak-default" {
		t.Fatalf("ResolveBrowserToTarget = %q, want cloak-default", got)
	}
}

func TestResolveBrowserToTarget_AmbiguousNoDefault(t *testing.T) {
	cfg := &RuntimeConfig{
		Targets: BrowserTargetsConfig{
			"cloak-eu": {Provider: BrowserCloak},
			"cloak-us": {Provider: BrowserCloak},
		},
	}
	_, err := ResolveBrowserToTarget(cfg, "cloak")
	if err == nil {
		t.Fatal("expected AmbiguousBrowserError, got nil")
	}
	var ambErr *AmbiguousBrowserError
	if !errors.As(err, &ambErr) {
		t.Fatalf("expected *AmbiguousBrowserError, got %T: %v", err, err)
	}
	if ambErr.Browser != "cloak" {
		t.Fatalf("AmbiguousBrowserError.Browser = %q, want cloak", ambErr.Browser)
	}
	if len(ambErr.Targets) != 2 || ambErr.Targets[0] != "cloak-eu" || ambErr.Targets[1] != "cloak-us" {
		t.Fatalf("AmbiguousBrowserError.Targets = %v, want [cloak-eu cloak-us]", ambErr.Targets)
	}
}

func TestResolveBrowserToTarget_AmbiguousWithDefault(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "cloak-eu",
		Targets: BrowserTargetsConfig{
			"cloak-eu": {Provider: BrowserCloak},
			"cloak-us": {Provider: BrowserCloak},
		},
	}
	got, err := ResolveBrowserToTarget(cfg, "cloak")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "cloak-eu" {
		t.Fatalf("ResolveBrowserToTarget = %q, want cloak-eu", got)
	}
}

func TestResolveBrowserToTarget_NoTargets(t *testing.T) {
	cfg := &RuntimeConfig{}
	got, err := ResolveBrowserToTarget(cfg, "chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveBrowserToTarget = %q, want empty", got)
	}
}

func TestResolveBrowserToTarget_NilConfig(t *testing.T) {
	got, err := ResolveBrowserToTarget(nil, "chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveBrowserToTarget = %q, want empty", got)
	}
}

func TestResolveBrowserToTarget_DefaultNotInMatches(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "chrome-local",
		Targets: BrowserTargetsConfig{
			"cloak-eu":     {Provider: BrowserCloak},
			"cloak-us":     {Provider: BrowserCloak},
			"chrome-local": {Provider: BrowserChrome},
		},
	}
	_, err := ResolveBrowserToTarget(cfg, "cloak")
	if err == nil {
		t.Fatal("expected AmbiguousBrowserError when default doesn't match browser, got nil")
	}
	var ambErr *AmbiguousBrowserError
	if !errors.As(err, &ambErr) {
		t.Fatalf("expected *AmbiguousBrowserError, got %T: %v", err, err)
	}
	if ambErr.Browser != "cloak" {
		t.Fatalf("AmbiguousBrowserError.Browser = %q, want cloak", ambErr.Browser)
	}
}

func TestResolveBrowserToTarget_NoMatchingProvider(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultTarget: "chrome-local",
		Targets: BrowserTargetsConfig{
			"chrome-local": {Provider: BrowserChrome},
		},
	}
	got, err := ResolveBrowserToTarget(cfg, "cloak")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveBrowserToTarget = %q, want empty (no matching provider)", got)
	}
}

func TestIsValidBrowserTargetName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"chrome", true},
		{"chrome-local", true},
		{"a", true},
		{"a1", true},
		{"a-b-c-1", true},
		{"", false},
		{"Chrome", false},
		{"1chrome", false},
		{"-chrome", false},
		{"chrome_local", false},
		{"chrome.local", false},
		{strings.Repeat("a", 33), false},
		{strings.Repeat("a", 32), true},
	}
	for _, tc := range cases {
		if got := IsValidBrowserTargetName(tc.name); got != tc.ok {
			t.Errorf("IsValidBrowserTargetName(%q) = %v, want %v", tc.name, got, tc.ok)
		}
	}
}

func TestFileConfig_TargetsRoundTrip(t *testing.T) {
	fc := FileConfig{
		Browser: BrowserConfig{
			Provider:      BrowserChrome,
			ChromeBinary:  "/usr/bin/chrome",
			DefaultTarget: "chrome-local",
			FallbackOrder: []string{"chrome-local"},
			Targets: BrowserTargetsConfig{
				"chrome-local": {Provider: BrowserChrome, Binary: "/usr/bin/chrome"},
			},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"targets"`) {
		t.Fatalf("marshal output missing targets block:\n%s", data)
	}
	if !strings.Contains(string(data), `"defaultTarget":"chrome-local"`) {
		t.Fatalf("marshal output missing defaultTarget:\n%s", data)
	}
	var back FileConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Browser.DefaultTarget != "chrome-local" {
		t.Fatalf("DefaultTarget lost: %q", back.Browser.DefaultTarget)
	}
	if _, ok := back.Browser.Targets["chrome-local"]; !ok {
		t.Fatalf("Targets[chrome-local] lost; got %+v", back.Browser.Targets)
	}
	// Legacy fields preserved.
	if back.Browser.Provider != BrowserChrome || back.Browser.ChromeBinary != "/usr/bin/chrome" {
		t.Fatalf("legacy fields lost: %+v", back.Browser)
	}
}

func TestFileConfig_LegacyOnlyMarshalOmitsNewKeys(t *testing.T) {
	fc := FileConfig{
		Browser: BrowserConfig{
			Provider:     BrowserChrome,
			ChromeBinary: "/usr/bin/chrome",
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, k := range []string{`"targets"`, `"defaultTarget"`, `"fallbackOrder"`} {
		if strings.Contains(s, k) {
			t.Errorf("legacy-only config emitted %s in JSON:\n%s", k, s)
		}
	}
}

func baseRuntimeForOverride() *RuntimeConfig {
	return &RuntimeConfig{
		DefaultBrowser:   BrowserChrome,
		ChromeBinary:     "/global/chrome",
		ChromeExtraFlags: "--global-flag",
		Cloak: CloakBrowserRuntimeConfig{
			FingerprintSeed: "global-seed",
			Platform:        "linux",
			Locale:          "en-US",
			StorageQuotaMB:  100,
		},
	}
}

func TestApplyTargetOverride_EmptyTargetName_ReturnsUnchanged(t *testing.T) {
	cfg := baseRuntimeForOverride()
	got := ApplyTargetOverride(cfg, "")
	if got != cfg {
		t.Fatalf("expected same pointer back for empty target name")
	}
}

func TestApplyTargetOverride_UnknownTarget_ReturnsUnchanged(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"chrome": BrowserTargetConfig{Provider: BrowserChrome},
	}
	got := ApplyTargetOverride(cfg, "missing")
	if got != cfg {
		t.Fatalf("expected same pointer back for unknown target")
	}
}

func TestApplyTargetOverride_NilCfg_ReturnsNil(t *testing.T) {
	if got := ApplyTargetOverride(nil, "anything"); got != nil {
		t.Fatalf("expected nil for nil cfg")
	}
}

func TestApplyTargetOverride_OverridesProviderBinaryFlags(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"cloak-eu": BrowserTargetConfig{
			Provider:   BrowserCloak,
			Binary:     "/opt/cloak/cloakbrowser",
			ExtraFlags: "--target-flag",
		},
	}
	got := ApplyTargetOverride(cfg, "cloak-eu")
	if got == cfg {
		t.Fatalf("expected a new pointer, got input pointer back")
	}
	if got.DefaultBrowser != BrowserCloak {
		t.Errorf("provider = %q, want %q", got.DefaultBrowser, BrowserCloak)
	}
	if got.ChromeBinary != "/opt/cloak/cloakbrowser" {
		t.Errorf("binary = %q, want /opt/cloak/cloakbrowser", got.ChromeBinary)
	}
	if got.ChromeExtraFlags != "--target-flag" {
		t.Errorf("extraFlags = %q, want --target-flag", got.ChromeExtraFlags)
	}
	// Input MUST be untouched.
	if cfg.DefaultBrowser != BrowserChrome ||
		cfg.ChromeBinary != "/global/chrome" ||
		cfg.ChromeExtraFlags != "--global-flag" {
		t.Errorf("input cfg was mutated: %+v", cfg)
	}
}

func TestApplyTargetOverride_EmptyBinary_KeepsGlobalBinary(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
			// no Binary, no ExtraFlags
		},
	}
	got := ApplyTargetOverride(cfg, "cloak")
	if got.ChromeBinary != "/global/chrome" {
		t.Errorf("binary = %q, want global /global/chrome (inherit)", got.ChromeBinary)
	}
	if got.ChromeExtraFlags != "--global-flag" {
		t.Errorf("extraFlags = %q, want global --global-flag (inherit)", got.ChromeExtraFlags)
	}
	if got.DefaultBrowser != BrowserCloak {
		t.Errorf("provider = %q, want cloak", got.DefaultBrowser)
	}
}

func TestApplyTargetOverride_CloakDeepMerge_PartialTarget(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
			Cloak: CloakBrowserConfig{
				// Override seed + timezone; leave others zero.
				FingerprintSeed: "target-seed",
				Timezone:        "Europe/London",
			},
		},
	}
	got := ApplyTargetOverride(cfg, "cloak")
	if got.Cloak.FingerprintSeed != "target-seed" {
		t.Errorf("FingerprintSeed = %q, want target-seed", got.Cloak.FingerprintSeed)
	}
	if got.Cloak.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q, want Europe/London", got.Cloak.Timezone)
	}
	// Preserved from global.
	if got.Cloak.Platform != "linux" {
		t.Errorf("Platform = %q, want preserved linux", got.Cloak.Platform)
	}
	if got.Cloak.Locale != "en-US" {
		t.Errorf("Locale = %q, want preserved en-US", got.Cloak.Locale)
	}
	if got.Cloak.StorageQuotaMB != 100 {
		t.Errorf("StorageQuotaMB = %d, want preserved 100", got.Cloak.StorageQuotaMB)
	}
	// Input still untouched.
	if cfg.Cloak.FingerprintSeed != "global-seed" || cfg.Cloak.Timezone != "" {
		t.Errorf("input cloak mutated: %+v", cfg.Cloak)
	}
}

func TestApplyTargetOverride_CloakEmpty_PreservesGlobalCloak(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
			// Cloak left zero.
		},
	}
	got := ApplyTargetOverride(cfg, "cloak")
	if got.Cloak != cfg.Cloak {
		t.Errorf("expected cloak unchanged from global; got %+v want %+v", got.Cloak, cfg.Cloak)
	}
}

func TestApplyTargetOverride_CloakUnion_NonOverlappingFields(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Cloak = CloakBrowserRuntimeConfig{FingerprintSeed: "g"}
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
			Cloak:    CloakBrowserConfig{Platform: "windows"},
		},
	}
	got := ApplyTargetOverride(cfg, "cloak")
	if got.Cloak.FingerprintSeed != "g" {
		t.Errorf("FingerprintSeed = %q, want g", got.Cloak.FingerprintSeed)
	}
	if got.Cloak.Platform != "windows" {
		t.Errorf("Platform = %q, want windows", got.Cloak.Platform)
	}
}

func TestApplyTargetOverride_CloakOverlap_TargetWins(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Cloak = CloakBrowserRuntimeConfig{
		FingerprintSeed:           "global-seed",
		StorageQuotaMB:            100,
		DisableDefaultStealthArgs: false,
	}
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
			Cloak: CloakBrowserConfig{
				FingerprintSeed:           "target-seed",
				StorageQuotaMB:            intPtr(2048),
				DisableDefaultStealthArgs: boolPtr(true),
			},
		},
	}
	got := ApplyTargetOverride(cfg, "cloak")
	if got.Cloak.FingerprintSeed != "target-seed" {
		t.Errorf("FingerprintSeed = %q, want target-seed", got.Cloak.FingerprintSeed)
	}
	if got.Cloak.StorageQuotaMB != 2048 {
		t.Errorf("StorageQuotaMB = %d, want 2048", got.Cloak.StorageQuotaMB)
	}
	if !got.Cloak.DisableDefaultStealthArgs {
		t.Errorf("DisableDefaultStealthArgs = false, want true")
	}
	// Input cloak still intact.
	if cfg.Cloak.FingerprintSeed != "global-seed" || cfg.Cloak.StorageQuotaMB != 100 ||
		cfg.Cloak.DisableDefaultStealthArgs {
		t.Errorf("input cloak mutated: %+v", cfg.Cloak)
	}
}

func TestApplyTargetOverride_ClonesTargetProxyAndTargetsMap(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Proxy = BrowserProxyConfig{
		Server:     "http://global.example:8080",
		BypassList: []string{"global"},
	}
	cfg.Targets = BrowserTargetsConfig{
		"proxy": {
			Provider: BrowserChrome,
			Proxy: BrowserProxyConfig{
				Server:     "http://proxy.example:8080",
				BypassList: []string{"localhost"},
				Geo:        &BrowserProxyGeoConfig{Timezone: "UTC"},
			},
		},
	}

	got := ApplyTargetOverride(cfg, "proxy")
	got.Proxy.BypassList[0] = "mutated"
	got.Proxy.Geo.Timezone = "Europe/London"
	got.Targets["proxy"] = BrowserTargetConfig{Provider: BrowserCloak}

	original := cfg.Targets["proxy"]
	if original.Proxy.BypassList[0] != "localhost" {
		t.Fatalf("target proxy bypass mutated through override: %+v", original.Proxy.BypassList)
	}
	if original.Proxy.Geo.Timezone != "UTC" {
		t.Fatalf("target proxy geo mutated through override: %+v", original.Proxy.Geo)
	}
	if original.Provider != BrowserChrome {
		t.Fatalf("target map shared with override: %+v", original)
	}
}
