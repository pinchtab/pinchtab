package config

import (
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)

func TestMigrateLegacyBrowserConfig_LegacyOnlySynthesizesDefaultTarget(t *testing.T) {
	bc := BrowserConfig{
		Provider:      BrowserChrome,
		BrowserBinary: "/usr/bin/chrome",
	}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc, "")
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
		Provider:      BrowserCloak,
		BrowserBinary: "/opt/cloak/chrome",
		Cloak: CloakBrowserConfig{
			FingerprintSeed: "42",
			Platform:        "linux",
		},
	}
	synthesized, _ := migrateLegacyBrowserConfig(&bc, "")
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
	synthesized, _ := migrateLegacyBrowserConfig(&bc, "")
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
	synthesized, conflict := migrateLegacyBrowserConfig(&bc, "")
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
		BrowserBinary: "/legacy/chrome",
		DefaultTarget: "explicit",
		Targets: BrowserTargetsConfig{
			"explicit": {Provider: BrowserChrome, Binary: "/explicit/chrome"},
		},
	}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc, "")
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
	if bc.Provider != BrowserChrome || bc.BrowserBinary != "/legacy/chrome" {
		t.Fatalf("legacy fields lost; got Provider=%q Binary=%q", bc.Provider, bc.BrowserBinary)
	}
}

func TestMigrateLegacyBrowserConfig_EmptyConfigNoop(t *testing.T) {
	bc := BrowserConfig{}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc, "")
	if synthesized || conflict {
		t.Fatalf("empty browser config should be a no-op; got synth=%v conflict=%v", synthesized, conflict)
	}
	if len(bc.Targets) != 0 || bc.DefaultTarget != "" {
		t.Fatalf("empty config mutated: %+v", bc)
	}
}

func TestMigrateLegacyBrowserConfig_CloakWithBrowsersDefault(t *testing.T) {
	bc := BrowserConfig{
		BrowserBinary: "/opt/cloakbrowser/chrome",
		Cloak: CloakBrowserConfig{
			FingerprintSeed: "42069",
			Platform:        "linux",
		},
	}
	synthesized, conflict := migrateLegacyBrowserConfig(&bc, "cloak")
	if !synthesized {
		t.Fatalf("expected synthesized=true")
	}
	if conflict {
		t.Fatalf("expected conflict=false")
	}
	tgt := bc.Targets[DefaultBrowserTargetName]
	if tgt.Provider != BrowserCloak {
		t.Fatalf("target.Provider = %q, want %q; browsers.default should be used when browser.provider is empty", tgt.Provider, BrowserCloak)
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
		BrowserBinary:     "/opt/cloak/chrome",
		DefaultTarget:     "cloak",
		BrowserExtraFlags: "--global-flag",
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
		DefaultBrowser:    BrowserChrome,
		BrowserBinary:     "/global/chrome",
		BrowserExtraFlags: "--global-flag",
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
	if resolved.Config.BrowserBinary != "/opt/cloak/chrome" {
		t.Fatalf("effective binary = %q, want /opt/cloak/chrome", resolved.Config.BrowserBinary)
	}
	if resolved.Config.BrowserExtraFlags != "--target-flag" {
		t.Fatalf("effective extraFlags = %q, want --target-flag", resolved.Config.BrowserExtraFlags)
	}
	if resolved.Config.Cloak.Timezone != "UTC" {
		t.Fatalf("effective cloak timezone = %q, want UTC", resolved.Config.Cloak.Timezone)
	}
	if cfg.DefaultBrowser != BrowserChrome || cfg.BrowserBinary != "/global/chrome" {
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
			BrowserBinary: "/usr/bin/chrome",
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
	if back.Browser.Provider != BrowserChrome || back.Browser.BrowserBinary != "/usr/bin/chrome" {
		t.Fatalf("legacy fields lost: %+v", back.Browser)
	}
}

func TestFileConfig_LegacyOnlyMarshalOmitsNewKeys(t *testing.T) {
	fc := FileConfig{
		Browser: BrowserConfig{
			Provider:      BrowserChrome,
			BrowserBinary: "/usr/bin/chrome",
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
		DefaultBrowser:    BrowserChrome,
		BrowserBinary:     "/global/chrome",
		BrowserExtraFlags: "--global-flag",
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
	if got.BrowserBinary != "/opt/cloak/cloakbrowser" {
		t.Errorf("binary = %q, want /opt/cloak/cloakbrowser", got.BrowserBinary)
	}
	if got.BrowserExtraFlags != "--target-flag" {
		t.Errorf("extraFlags = %q, want --target-flag", got.BrowserExtraFlags)
	}
	if cfg.DefaultBrowser != BrowserChrome ||
		cfg.BrowserBinary != "/global/chrome" ||
		cfg.BrowserExtraFlags != "--global-flag" {
		t.Errorf("input cfg was mutated: %+v", cfg)
	}
}

func TestApplyTargetOverride_EmptyBinary_KeepsGlobalBinary(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
		},
	}
	got := ApplyTargetOverride(cfg, "cloak")
	if got.BrowserBinary != "/global/chrome" {
		t.Errorf("binary = %q, want global /global/chrome (inherit)", got.BrowserBinary)
	}
	if got.BrowserExtraFlags != "--global-flag" {
		t.Errorf("extraFlags = %q, want global --global-flag (inherit)", got.BrowserExtraFlags)
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
	if got.Cloak.Platform != "linux" {
		t.Errorf("Platform = %q, want preserved linux", got.Cloak.Platform)
	}
	if got.Cloak.Locale != "en-US" {
		t.Errorf("Locale = %q, want preserved en-US", got.Cloak.Locale)
	}
	if got.Cloak.StorageQuotaMB != 100 {
		t.Errorf("StorageQuotaMB = %d, want preserved 100", got.Cloak.StorageQuotaMB)
	}
	if cfg.Cloak.FingerprintSeed != "global-seed" || cfg.Cloak.Timezone != "" {
		t.Errorf("input cloak mutated: %+v", cfg.Cloak)
	}
}

func TestApplyTargetOverride_CloakEmpty_PreservesGlobalCloak(t *testing.T) {
	cfg := baseRuntimeForOverride()
	cfg.Targets = BrowserTargetsConfig{
		"cloak": BrowserTargetConfig{
			Provider: BrowserCloak,
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

func TestApplyFileConfig_ExplicitDefaultTargetDrivesDefaultBrowser(t *testing.T) {
	fc := &FileConfig{
		Browser: BrowserConfig{
			Targets: BrowserTargetsConfig{
				DefaultBrowserTargetName: {
					Provider: BrowserCloak,
					Binary:   "/opt/cloak/bin",
					Proxy: BrowserProxyConfig{
						Server:   "http://proxy.example:8080",
						Username: "user",
						Password: "secret",
					},
				},
			},
		},
	}
	cfg := &RuntimeConfig{}
	applyFileConfig(cfg, fc)

	if cfg.DefaultBrowser != BrowserCloak {
		t.Fatalf("DefaultBrowser = %q, want %q (derived from explicit default target)", cfg.DefaultBrowser, BrowserCloak)
	}
	if cfg.TargetsSynthesized {
		t.Fatal("user-authored targets must not be marked synthesized")
	}

	out := FileConfigFromRuntime(cfg)
	got, ok := out.Browser.Targets[DefaultBrowserTargetName]
	if !ok {
		t.Fatalf("default target lost in round-trip: %+v", out.Browser.Targets)
	}
	if got.Provider != BrowserCloak {
		t.Fatalf("Provider = %q, want %q (user target must survive round-trip)", got.Provider, BrowserCloak)
	}
	if got.Binary != "/opt/cloak/bin" {
		t.Fatalf("Binary = %q, want /opt/cloak/bin", got.Binary)
	}
	if got.Proxy.Password != "secret" {
		t.Fatalf("proxy credentials wiped in round-trip: %+v", got.Proxy)
	}
}

func TestFileConfigFromRuntime_UserTargetSurvivesContradictoryDefaultBrowser(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{Default: BrowserChrome},
		Browser: BrowserConfig{
			Targets: BrowserTargetsConfig{
				DefaultBrowserTargetName: {
					Provider: BrowserCloak,
					Binary:   "/opt/cloak/bin",
					Proxy:    BrowserProxyConfig{Server: "http://proxy.example:8080", Username: "u", Password: "secret"},
				},
			},
		},
	}
	cfg := &RuntimeConfig{}
	applyFileConfig(cfg, fc)

	if cfg.DefaultBrowser != BrowserChrome {
		t.Fatalf("DefaultBrowser = %q, want explicit browsers.default to win", cfg.DefaultBrowser)
	}

	out := FileConfigFromRuntime(cfg)
	got := out.Browser.Targets[DefaultBrowserTargetName]
	if got.Provider != BrowserCloak || got.Binary != "/opt/cloak/bin" || got.Proxy.Password != "secret" {
		t.Fatalf("user-authored target rewritten despite contradictory browsers.default: %+v", got)
	}
}

func TestFileConfigFromRuntime_SynthesizedTargetStillReconciles(t *testing.T) {
	fc := &FileConfig{
		Browser: BrowserConfig{BrowserBinary: "/usr/bin/chrome"},
	}
	cfg := &RuntimeConfig{}
	applyFileConfig(cfg, fc)

	if !cfg.TargetsSynthesized {
		t.Fatal("legacy-only config should synthesize targets")
	}
	if cfg.DefaultBrowser != BrowserChrome {
		t.Fatalf("DefaultBrowser = %q, want chrome", cfg.DefaultBrowser)
	}

	// Orchestrator child-launch scenario: caller overrides the provider
	// without rewriting Targets; the synthesized target must follow.
	cfg.DefaultBrowser = BrowserCloak
	out := FileConfigFromRuntime(cfg)
	got := out.Browser.Targets[DefaultBrowserTargetName]
	if got.Provider != BrowserCloak {
		t.Fatalf("synthesized target Provider = %q, want reconciled to %q", got.Provider, BrowserCloak)
	}
}

func TestMatchBrowserToTarget(t *testing.T) {
	t.Run("nil cfg", func(t *testing.T) {
		if target, matches := MatchBrowserToTarget(nil, BrowserCloak); target != "" || matches != nil {
			t.Fatalf("got (%q, %v), want (\"\", nil)", target, matches)
		}
	})

	t.Run("single match wins", func(t *testing.T) {
		cfg := &RuntimeConfig{Targets: BrowserTargetsConfig{
			"only-cloak": {Provider: BrowserCloak},
			"a-chrome":   {Provider: BrowserChrome},
		}}
		target, matches := MatchBrowserToTarget(cfg, BrowserCloak)
		if target != "only-cloak" || len(matches) != 1 {
			t.Fatalf("got (%q, %v), want (only-cloak, 1 match)", target, matches)
		}
	})

	t.Run("zero matches", func(t *testing.T) {
		cfg := &RuntimeConfig{Targets: BrowserTargetsConfig{"a-chrome": {Provider: BrowserChrome}}}
		if target, matches := MatchBrowserToTarget(cfg, BrowserCloak); target != "" || len(matches) != 0 {
			t.Fatalf("got (%q, %v), want (\"\", 0 matches)", target, matches)
		}
	})

	t.Run("multiple matches prefer configured default", func(t *testing.T) {
		cfg := &RuntimeConfig{
			DefaultTarget: "cloak-b",
			Targets: BrowserTargetsConfig{
				"cloak-a": {Provider: BrowserCloak},
				"cloak-b": {Provider: BrowserCloak},
			},
		}
		target, matches := MatchBrowserToTarget(cfg, BrowserCloak)
		if target != "cloak-b" || len(matches) != 2 {
			t.Fatalf("got (%q, %v), want (cloak-b, 2 matches)", target, matches)
		}
	})

	t.Run("multiple matches no default among them is ambiguous", func(t *testing.T) {
		cfg := &RuntimeConfig{
			DefaultTarget: "a-chrome", // not a cloak target
			Targets: BrowserTargetsConfig{
				"cloak-a":  {Provider: BrowserCloak},
				"cloak-b":  {Provider: BrowserCloak},
				"a-chrome": {Provider: BrowserChrome},
			},
		}
		if target, matches := MatchBrowserToTarget(cfg, BrowserCloak); target != "" || len(matches) != 2 {
			t.Fatalf("got (%q, %v), want (\"\", 2 matches)", target, matches)
		}
	})
}
