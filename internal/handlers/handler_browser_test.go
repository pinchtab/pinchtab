package handlers

import (
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestResolveEffectiveConfig_WithTarget(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		ChromeBinary:   "/usr/bin/chrome",
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
	if effective.ChromeBinary != "/opt/cloakbrowser/chrome" {
		t.Errorf("expected ChromeBinary=%q, got %q", "/opt/cloakbrowser/chrome", effective.ChromeBinary)
	}
}

func TestResolveEffectiveConfig_NoTargets(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		ChromeBinary:   "/usr/bin/chrome",
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
