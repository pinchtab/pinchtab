package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplyingAFileThatSaysNothingLeavesEveryRuntimeDefaultInPlace(t *testing.T) {
	clearConfigEnvVars(t)
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "nonexistent.json"))

	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	before := FileConfigFromRuntime(cfg).Security

	ApplyFileConfigToRuntime(cfg, &FileConfig{})

	after := FileConfigFromRuntime(cfg).Security
	if !reflect.DeepEqual(before, after) {
		t.Errorf("a file with no security section changed the security defaults\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestAnAbsentIDPIBlockIsNotAnExplicitlyOffOne(t *testing.T) {
	live := IDPIConfig{Enabled: true, StrictMode: true, ScanTimeoutSec: 5}

	cfg := &RuntimeConfig{IDPI: live}
	ApplyFileConfigToRuntime(cfg, &FileConfig{})
	if !reflect.DeepEqual(cfg.IDPI, live) {
		t.Errorf("a file with no idpi block overwrote the runtime value: got %+v, want %+v", cfg.IDPI, live)
	}

	cfg = &RuntimeConfig{IDPI: live}
	ApplyFileConfigToRuntime(cfg, &FileConfig{Security: SecurityConfig{IDPI: &IDPIConfig{}}})
	if !reflect.DeepEqual(cfg.IDPI, IDPIConfig{}) {
		t.Errorf("an explicit all-off idpi block did not switch IDPI off: got %+v", cfg.IDPI)
	}
}

func TestASparseFileLoadsWithoutAnIDPIBlockWhileTheStarterStatesOne(t *testing.T) {
	clearConfigEnvVars(t)
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"server":{"port":"18799","token":"t"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	fc, _, err := LoadFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	if fc.Security.IDPI != nil {
		t.Errorf("a file that says nothing about IDPI loaded a block: %+v; the next save would install it as an explicit setting the boot never enforced", *fc.Security.IDPI)
	}

	starter := DefaultFileConfig().Security.IDPI
	if starter == nil || !starter.Enabled {
		t.Errorf("the starter file no longer states IDPI on: %+v", starter)
	}
}

func TestTheSecuritySectionComparesAnAbsentIDPIBlockEqualToAnAllOffOne(t *testing.T) {
	absent := SecurityConfig{}
	explicitOff := SecurityConfig{IDPI: &IDPIConfig{}}
	if !sameJSONValue(absent, explicitOff) {
		t.Error("an absent idpi block and an all-off one render differently, so a dashboard save that echoes the values in effect reads as a security edit")
	}
	on := SecurityConfig{IDPI: &IDPIConfig{Enabled: true}}
	if sameJSONValue(absent, on) {
		t.Error("an absent idpi block renders the same as an enabled one, so the comparison cannot see IDPI being switched on")
	}
}
