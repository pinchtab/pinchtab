package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileConfigAcceptsRetiredAllocationPolicyWithAnAdvisory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"multiInstance":{"strategy":"simple","allocationPolicy":"round_robin"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)

	result := readAndParseConfigFile()
	if result.ParseErr != nil || result.UnknownFields != nil || len(result.ValidationErrs) > 0 {
		t.Fatalf("legacy config did not load cleanly: parse=%v unknown=%v invalid=%v", result.ParseErr, result.UnknownFields, result.ValidationErrs)
	}
	if result.FC == nil || result.FC.MultiInstance.Strategy != "simple" {
		t.Fatalf("sibling keys were not kept: %+v", result.FC)
	}
	if len(result.Advisories) != 1 || !strings.Contains(result.Advisories[0], "multiInstance.allocationPolicy is retired") {
		t.Fatalf("advisories = %q, want one retirement advisory", result.Advisories)
	}
}

func TestSetConfigValueRefusesRetiredAllocationPolicy(t *testing.T) {
	fc := &FileConfig{}
	if err := SetConfigValue(fc, "multiInstance.allocationPolicy", "round_robin"); err == nil {
		t.Fatal("setting the retired key succeeded")
	}
}
