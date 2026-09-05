package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/output"
)

func customConfigWithDefaultPresent(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	defaultConfig := filepath.Join(home, ".pinchtab", "config.json")
	if err := os.MkdirAll(filepath.Dir(defaultConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(home, "elsewhere.json"))
	output.ResetAdvisories()
	t.Cleanup(output.ResetAdvisories)
}

// The default-config advisory used to be written straight to stderr by
// internal/config under a sync.Once of its own, so the one hint switch did not
// reach it: a caller who had silenced hints still got this one, forever.
func TestTheDefaultConfigAdvisoryObeysTheHintSwitch(t *testing.T) {
	customConfigWithDefaultPresent(t)
	t.Setenv(output.HintsEnv, output.HintsOff)

	if got := captureStderr(t, adviseDefaultConfig); strings.Contains(got, "HINT:") {
		t.Errorf("stderr = %q, want no hint: %s=%s silences advisories, and this one is no exception", got, output.HintsEnv, output.HintsOff)
	}
}

func TestTheDefaultConfigAdvisoryPrintsOncePerProcessWithTheSilencer(t *testing.T) {
	customConfigWithDefaultPresent(t)

	got := captureStderr(t, func() {
		adviseDefaultConfig()
		adviseDefaultConfig()
	})
	if count := strings.Count(got, "default config exists at"); count != 1 {
		t.Errorf("the advisory printed %d times across two config-touching commands, want 1: %q", count, got)
	}
	if !strings.Contains(got, output.SilenceAdvisoryHint) {
		t.Errorf("stderr = %q, want the silencer beside it so the switch is discoverable from the output it silences", got)
	}
}

func TestNoAdvisoryWhenTheCallerRunsAgainstTheDefaultConfig(t *testing.T) {
	customConfigWithDefaultPresent(t)
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(os.Getenv("HOME"), ".pinchtab", "config.json"))

	if got := captureStderr(t, adviseDefaultConfig); got != "" {
		t.Errorf("stderr = %q, want nothing: the advisory has nothing to say to a caller already on the default config", got)
	}
}
