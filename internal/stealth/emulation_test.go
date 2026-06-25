package stealth

import (
	"context"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ApplyTargetEmulation must self-defend on native Cloak: it returns nil before
// any CDP emulation override runs, so a background (non-chromedp) context is
// enough — no executor is needed because the guard short-circuits first.
func TestApplyTargetEmulation_SkipsNativeCloak(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed:           "42069",
			DisableDefaultStealthArgs: true,
		},
	}
	if !config.PinchTabStealthDefaultsDisabled(cfg) {
		t.Fatal("precondition: cfg should report PinchTab stealth defaults disabled")
	}
	if err := ApplyTargetEmulation(context.Background(), cfg, ""); err != nil {
		t.Fatalf("ApplyTargetEmulation on native cloak should no-op, got %v", err)
	}
}

func TestApplyTargetEmulation_NilConfig(t *testing.T) {
	if err := ApplyTargetEmulation(context.Background(), nil, ""); err != nil {
		t.Fatalf("nil cfg should no-op, got %v", err)
	}
}
