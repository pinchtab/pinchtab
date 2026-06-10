package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestResolveEffectiveConfig_WithTarget(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		BrowserBinary:  "/usr/bin/chrome",
		ActionTimeout:  10 * time.Second,
		Targets: config.BrowserTargetsConfig{
			"cloak-us": {
				Provider: config.BrowserCloak,
				Binary:   "/opt/cloakbrowser/chrome",
			},
		},
		DefaultTarget: "cloak-us",
	}
	h := &Handlers{Config: cfg}

	// Pass the browser name "cloak" — the function resolves internally to
	// the "cloak-us" target whose provider is "cloak".
	effective, err := h.resolveEffectiveConfig("cloak")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if effective == cfg {
		t.Fatal("expected a new config, got same pointer as h.Config")
	}
	if effective.DefaultBrowser != config.BrowserCloak {
		t.Errorf("expected DefaultBrowser=%q, got %q", config.BrowserCloak, effective.DefaultBrowser)
	}
	if effective.BrowserBinary != "/opt/cloakbrowser/chrome" {
		t.Errorf("expected BrowserBinary=%q, got %q", "/opt/cloakbrowser/chrome", effective.BrowserBinary)
	}
}

func TestResolveEffectiveConfig_NoTargets(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		BrowserBinary:  "/usr/bin/chrome",
	}
	h := &Handlers{Config: cfg}

	effective, err := h.resolveEffectiveConfig("anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if effective != cfg {
		t.Fatal("expected same config pointer when no targets are configured")
	}
}

func TestResolveEffectiveConfig_EmptyBrowserName(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		Targets: config.BrowserTargetsConfig{
			"default": {Provider: config.BrowserChrome},
		},
		DefaultTarget: "default",
	}
	h := &Handlers{Config: cfg}

	effective, err := h.resolveEffectiveConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if effective != cfg {
		t.Fatal("expected same config pointer when browser name is empty")
	}
}

func TestResolveEffectiveConfig_NoMatchingProvider(t *testing.T) {
	// Only cloak targets configured — passing "chrome" should find no match
	// because no target has Provider=chrome.
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		Targets: config.BrowserTargetsConfig{
			"cloak-only": {Provider: config.BrowserCloak},
		},
		DefaultTarget: "cloak-only",
	}
	h := &Handlers{Config: cfg}

	effective, err := h.resolveEffectiveConfig("chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if effective != cfg {
		t.Fatal("expected same config pointer (fallback) when no targets match")
	}
}

func TestResolveEffectiveConfig_AmbiguousBrowser(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		Targets: config.BrowserTargetsConfig{
			"cloak-eu": {Provider: config.BrowserCloak},
			"cloak-us": {Provider: config.BrowserCloak},
		},
	}
	h := &Handlers{Config: cfg}

	_, err := h.resolveEffectiveConfig("cloak")
	if err == nil {
		t.Fatal("expected AmbiguousBrowserError, got nil")
	}
	ambErr, ok := err.(*config.AmbiguousBrowserError)
	if !ok {
		t.Fatalf("expected *AmbiguousBrowserError, got %T: %v", err, err)
	}
	if ambErr.Browser != "cloak" {
		t.Fatalf("AmbiguousBrowserError.Browser = %q, want cloak", ambErr.Browser)
	}
}

// M6 regression: a target that exists but fails to resolve must surface an
// error — silently substituting the global config would run the request with
// the wrong binary/proxy/fingerprint.
func TestResolveEffectiveConfig_BrokenTargetErrorsInsteadOfGlobalFallback(t *testing.T) {
	// "Bad-Name" violates the target-name grammar (^[a-z][a-z0-9-]{0,31}$),
	// which loads with only a warning but fails explicit resolution.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		Targets: config.BrowserTargetsConfig{
			"Bad-Name": {Provider: config.BrowserCloak, Binary: "/opt/cloak/bin"},
		},
	}, nil, nil, nil)

	cfg, err := h.resolveEffectiveConfig(config.BrowserCloak)
	if err == nil {
		t.Fatalf("expected resolution error for broken target, got cfg=%+v", cfg)
	}
	if !strings.Contains(err.Error(), "Bad-Name") {
		t.Fatalf("error should name the broken target: %v", err)
	}
}

// The legitimate fallbacks stay: no target configured for the provider means
// the global config, not an error.
func TestResolveEffectiveConfig_NoTargetForProviderFallsBack(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		Targets: config.BrowserTargetsConfig{
			"cloak-1": {Provider: config.BrowserCloak},
		},
	}, nil, nil, nil)

	cfg, err := h.resolveEffectiveConfig(config.BrowserChrome)
	if err != nil {
		t.Fatalf("no-target-for-provider should fall back, got error: %v", err)
	}
	if cfg != h.Config {
		t.Fatal("expected the global config on fallback")
	}
}
