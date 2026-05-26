package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/spf13/cobra"
)

func TestRunDoctorBrowserShowsOverviewForUnknownTarget(t *testing.T) {
	writeDoctorTestConfig(t, func(fc *config.FileConfig) {
		fc.Browser.Targets = config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
		}
		fc.Browser.DefaultTarget = "chrome"
	})

	var out bytes.Buffer
	err := runDoctorBrowser(doctorTestCommand(&out), []string{"nonexistent"})
	if err != nil {
		t.Fatalf("runDoctorBrowser() unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "not a known provider") {
		t.Fatalf("expected unknown browser message, got %q", output)
	}
	if !strings.Contains(output, "Known browsers:") {
		t.Fatalf("expected known browsers list, got %q", output)
	}
}

func TestRunDoctorBrowserNoArgsShowsOverview(t *testing.T) {
	writeDoctorTestConfig(t, nil)

	var out bytes.Buffer
	err := runDoctorBrowser(doctorTestCommand(&out), nil)
	if err != nil {
		t.Fatalf("runDoctorBrowser() unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Supported browsers:") {
		t.Fatalf("expected overview output, got %q", output)
	}
	if !strings.Contains(output, "chrome") {
		t.Fatalf("expected chrome in output, got %q", output)
	}
}

func TestRunDoctorReturnsUsageErrorWithoutMutatingCheckFlag(t *testing.T) {
	writeDoctorTestConfig(t, nil)
	restore := setDoctorGlobals(" nope ", false)
	t.Cleanup(restore)

	var out bytes.Buffer
	err := runDoctor(doctorTestCommand(&out), nil)
	if err == nil {
		t.Fatal("runDoctor() expected error")
	}
	if got := commandExitCode(err); got != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v", got, err)
	}
	if !strings.Contains(err.Error(), `unknown check "nope"`) {
		t.Fatalf("expected unknown check error, got %q", err.Error())
	}
	if doctorCheck != " nope " {
		t.Fatalf("doctorCheck mutated to %q", doctorCheck)
	}
}

func TestRunDoctorReturnsFailureCodeWithoutExiting(t *testing.T) {
	missingBinary := filepath.Join(t.TempDir(), "missing-browser")
	writeDoctorTestConfig(t, func(fc *config.FileConfig) {
		fc.Browser.ChromeBinary = missingBinary
	})
	restore := setDoctorGlobals("binary_exists", false)
	t.Cleanup(restore)

	var out bytes.Buffer
	err := runDoctor(doctorTestCommand(&out), nil)
	if err == nil {
		t.Fatal("runDoctor() expected error")
	}
	if got := commandExitCode(err); got != 1 {
		t.Fatalf("exit code = %d, want 1; err=%v", got, err)
	}
	if !strings.Contains(out.String(), "binary_exists") {
		t.Fatalf("expected doctor output to include binary_exists, got %q", out.String())
	}
	if doctorCheck != "binary_exists" {
		t.Fatalf("doctorCheck mutated to %q", doctorCheck)
	}
}

func writeDoctorTestConfig(t *testing.T, mutate func(*config.FileConfig)) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	if mutate != nil {
		mutate(&fc)
	}
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}
}

func setDoctorGlobals(check string, json bool) func() {
	oldCheck, oldJSON := doctorCheck, doctorJSON
	doctorCheck = check
	doctorJSON = json
	return func() {
		doctorCheck = oldCheck
		doctorJSON = oldJSON
	}
}

func doctorTestCommand(out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetContext(context.Background())
	return cmd
}
