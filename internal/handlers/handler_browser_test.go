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

	effective := h.resolveEffectiveConfig("cloak-us")

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

	effective := h.resolveEffectiveConfig("anything")

	if effective != cfg {
		t.Fatal("expected same config pointer when no targets are configured")
	}
}

func TestResolveEffectiveConfig_EmptyTargetName(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		Targets: config.BrowserTargetsConfig{
			"default": {Provider: config.BrowserChrome},
		},
		DefaultTarget: "default",
	}
	h := &Handlers{Config: cfg}

	effective := h.resolveEffectiveConfig("")

	if effective != cfg {
		t.Fatal("expected same config pointer when target name is empty")
	}
}

func TestResolveEffectiveConfig_UnknownTarget(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		Targets: config.BrowserTargetsConfig{
			"default": {Provider: config.BrowserChrome},
		},
		DefaultTarget: "default",
	}
	h := &Handlers{Config: cfg}

	effective := h.resolveEffectiveConfig("nonexistent")

	if effective != cfg {
		t.Fatal("expected same config pointer (fallback) when target is unknown")
	}
}
