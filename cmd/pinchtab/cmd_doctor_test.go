package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/spf13/cobra"
)

func TestRunDoctorReturnsUsageErrorWithoutMutatingTargetFlag(t *testing.T) {
	writeDoctorTestConfig(t, func(fc *config.FileConfig) {
		fc.Browser.Targets = config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserProviderChrome},
		}
		fc.Browser.DefaultTarget = "chrome"
	})
	restore := setDoctorGlobals(" missing ", " ", false)
	t.Cleanup(restore)

	var out bytes.Buffer
	err := runDoctor(doctorTestCommand(&out), nil)
	if err == nil {
		t.Fatal("runDoctor() expected error")
	}
	if got := commandExitCode(err); got != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v", got, err)
	}
	if !strings.Contains(err.Error(), `target "missing" not found`) {
		t.Fatalf("expected missing target error, got %q", err.Error())
	}
	if doctorTarget != " missing " {
		t.Fatalf("doctorTarget mutated to %q", doctorTarget)
	}
	if doctorCheck != " " {
		t.Fatalf("doctorCheck mutated to %q", doctorCheck)
	}
}

func TestRunDoctorReturnsUsageErrorWithoutMutatingCheckFlag(t *testing.T) {
	writeDoctorTestConfig(t, nil)
	restore := setDoctorGlobals("", " nope ", false)
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
	restore := setDoctorGlobals("", "binary_exists", false)
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

func setDoctorGlobals(target, check string, json bool) func() {
	oldTarget, oldCheck, oldJSON := doctorTarget, doctorCheck, doctorJSON
	doctorTarget = target
	doctorCheck = check
	doctorJSON = json
	return func() {
		doctorTarget = oldTarget
		doctorCheck = oldCheck
		doctorJSON = oldJSON
	}
}

func doctorTestCommand(out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	return cmd
}
