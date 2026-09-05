package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// The guards-down preset used to hand-list its capability toggles and had silently drifted
// from the canonical table — omitting stateExport. It now derives from routes, so this census
// walks CapabilityEndpoints() and drives the real preset: every gated capability must be
// enabled EXCEPT the one recorded exclusion, and each must resolve through routes.Meta rather
// than a synthesised path. A capability that gates routes but routes.Meta cannot describe
// (an endpoint added without a table entry) fails here rather than being silently skipped.
func TestGuardsDownEnablesEveryCapabilityExceptTheRecordedExclusion(t *testing.T) {
	gated := routes.CapabilityEndpoints()
	if len(gated) < 2 {
		t.Fatalf("only %d gated capabilities; this census would pass vacuously", len(gated))
	}

	fc := config.DefaultFileConfig()
	if err := BuildGuardsDownConfig(&fc); err != nil {
		t.Fatalf("BuildGuardsDownConfig() error = %v", err)
	}

	excludedSeen := false
	for cap := range gated {
		meta, ok := routes.Meta(cap)
		if !ok {
			t.Errorf("capability %q gates routes but routes.Meta does not describe it, so guards-down cannot derive its setting", cap)
			continue
		}
		got, err := config.GetConfigValue(&fc, meta.Setting)
		if err != nil {
			t.Errorf("reading %s for capability %q: %v", meta.Setting, cap, err)
			continue
		}
		if cap == guardsDownExcludedCapability {
			excludedSeen = true
			if got == "true" {
				t.Errorf("guards-down enabled the recorded-exclusion capability %q (%s); disk state export must stay a deliberate opt-out, not a side effect of turning guards off", cap, meta.Setting)
			}
			continue
		}
		if got != "true" {
			t.Errorf("guards-down left capability %q (%s) at %q, not enabled; a capability it omits is silently kept off", cap, meta.Setting, got)
		}
	}
	if !excludedSeen {
		t.Fatalf("the recorded exclusion %q is not among the gated capabilities; the exclusion is stale", guardsDownExcludedCapability)
	}
}

func TestApplyGuardsDownPreset(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = "guarded-token"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	result, err := ApplyGuardsDownPreset(false)
	if err != nil {
		t.Fatalf("ApplyGuardsDownPreset() error = %v", err)
	}
	if !result.Written {
		t.Fatal("expected guards down preset to change config")
	}
	if result.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", result.ConfigPath, configPath)
	}
	cfg := config.Load()

	if cfg.Bind != "127.0.0.1" {
		t.Fatalf("Bind = %q, want 127.0.0.1", cfg.Bind)
	}
	if cfg.Token != "guarded-token" {
		t.Fatalf("Token = %q, want existing token to remain", cfg.Token)
	}
	if !cfg.AllowEvaluate || !cfg.AllowMacro || !cfg.AllowScreencast || !cfg.AllowDownload || !cfg.AllowCookies || !cfg.AllowUpload {
		t.Fatalf("expected sensitive endpoints enabled, got %+v", cfg)
	}
	if !cfg.AttachEnabled {
		t.Fatal("expected attach endpoint enabled")
	}
	if got := strings.Join(cfg.AttachAllowHosts, ","); got != "127.0.0.1,localhost,::1" {
		t.Fatalf("AttachAllowHosts = %q", got)
	}
	if got := strings.Join(cfg.AttachAllowSchemes, ","); got != "ws,wss" {
		t.Fatalf("AttachAllowSchemes = %q", got)
	}
	if cfg.IDPI.Enabled || cfg.IDPI.StrictMode || cfg.IDPI.ScanContent || cfg.IDPI.WrapContent {
		t.Fatalf("expected IDPI protections disabled, got %+v", cfg.IDPI)
	}
}

func TestGuardsDownPostureActiveMirrorsThePreset(t *testing.T) {
	fc := config.DefaultFileConfig()
	if err := BuildGuardsDownConfig(&fc); err != nil {
		t.Fatalf("BuildGuardsDownConfig() error = %v", err)
	}
	if !GuardsDownPostureActive(config.NextRuntimeConfig(config.Load(), &fc)) {
		t.Fatal("GuardsDownPostureActive() = false right after applying the preset")
	}
	if GuardsDownPostureActive(config.NextRuntimeConfig(config.Load(), ptr(config.DefaultFileConfig()))) {
		t.Fatal("GuardsDownPostureActive() = true for the default config")
	}
	for cap := range routes.CapabilityEndpoints() {
		if cap == guardsDownExcludedCapability {
			continue
		}
		meta, _ := routes.Meta(cap)
		relaxed := fc
		if err := config.SetConfigValue(&relaxed, meta.Setting, "false"); err != nil {
			t.Fatalf("set %s: %v", meta.Setting, err)
		}
		if GuardsDownPostureActive(config.NextRuntimeConfig(config.Load(), &relaxed)) {
			t.Errorf("GuardsDownPostureActive() = true with capability %q disabled; the detector must track every preset capability", cap)
		}
	}
}

func ptr[T any](v T) *T { return &v }
