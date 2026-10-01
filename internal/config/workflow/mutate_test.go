package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestInitDefaultConfigIncludesSchema(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")

	if err := InitDefaultConfig(configPath); err != nil {
		t.Fatalf("InitDefaultConfig() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("config JSON is invalid: %v", err)
	}
	if raw["$schema"] != config.CurrentConfigSchemaURL() {
		t.Fatalf("$schema = %q, want %q", raw["$schema"], config.CurrentConfigSchemaURL())
	}
}

func TestValidateCurrentFileWarnsOnRetiredKeys(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"multiInstance": {"strategy": "simple", "allocationPolicy": "fcfs"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", configPath)

	_, errs, advisories, err := ValidateCurrentFile()
	if err != nil {
		t.Fatalf("ValidateCurrentFile() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("validation errors = %v, want none", errs)
	}
	if !slices.ContainsFunc(advisories, func(a string) bool {
		return strings.HasPrefix(a, "multiInstance.allocationPolicy is retired")
	}) {
		t.Fatalf("advisories = %q, want the allocationPolicy retirement advisory", advisories)
	}
}
