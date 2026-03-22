package stealth

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestNewBundleIncludesSeedLevelAndPopupGuard(t *testing.T) {
	bundle := NewBundle(&config.RuntimeConfig{StealthLevel: "medium"}, 1234)
	if bundle == nil {
		t.Fatal("expected non-nil bundle")
	}
	if bundle.Level != LevelMedium {
		t.Fatalf("expected level medium, got %s", bundle.Level)
	}
	for _, want := range []string{
		"var __pinchtab_seed = 1234;",
		`var __pinchtab_stealth_level = "medium";`,
		"window.open",
		"window.opener",
	} {
		if !strings.Contains(bundle.Script, want) {
			t.Fatalf("expected bundle script to contain %q", want)
		}
	}
	if !strings.HasPrefix(bundle.ScriptHash, "sha256:") {
		t.Fatalf("expected script hash prefix, got %q", bundle.ScriptHash)
	}
}

func TestStatusFromBundleReflectsCurrentCapabilityShape(t *testing.T) {
	cfg := &config.RuntimeConfig{StealthLevel: "full", Headless: true}
	bundle := NewBundle(cfg, 7)
	status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if !status.Capabilities["webglSpoofing"] {
		t.Fatal("expected full mode to report webgl spoofing")
	}
	if status.Capabilities["webdriverNativeStrategy"] {
		t.Fatal("expected current status to report non-native webdriver strategy")
	}
	if !status.Flags["headlessNew"] {
		t.Fatal("expected headlessNew flag to be true for headless config")
	}
}
