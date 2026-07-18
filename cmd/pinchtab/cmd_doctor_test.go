package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/doctor"
	"github.com/spf13/cobra"
)

// M7 regression: an unknown target/browser must return an error (non-zero
// exit) so scripts can rely on the documented exit contract, while a bare
// KNOWN browser ID still gets the focused overview.
func TestRunDoctorBrowserUnknownTargetReturnsError(t *testing.T) {
	writeDoctorTestConfig(t, func(fc *config.FileConfig) {
		fc.Browser.Targets = config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
		}
		fc.Browser.DefaultTarget = "chrome"
	})

	var out bytes.Buffer
	err := runDoctorBrowser(doctorTestCommand(&out), []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown browser/target")
	}
	var coded *commandExitError
	if !errors.As(err, &coded) || coded.ExitCode() == 0 {
		t.Fatalf("expected non-zero commandExitError, got %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error should name the unknown target: %v", err)
	}

	out.Reset()
	if err := runDoctorBrowser(doctorTestCommand(&out), []string{"cloak"}); err != nil {
		t.Fatalf("bare known browser ID should show the focused overview: %v", err)
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

func TestBrowserOverviewCloakHintsMatchPresenceState(t *testing.T) {
	base := doctor.BrowserInfo{
		Name:       "cloak",
		Registered: true,
		Status:     "needs-config",
	}

	missingConfig := base
	missingConfig.Checks = []doctor.CheckResult{{
		Name:   "cloakbrowser_present",
		Status: doctor.StatusWarn,
		Detail: "CloakBrowser found at /opt/cloak/chrome -> 130.0.0, but browser.binary is unset",
	}}
	if hint := browserOverviewHint(missingConfig); !strings.Contains(hint, "Set browser.binary") {
		t.Fatalf("unset binary hint = %q, want config guidance", hint)
	}

	brokenPath := base
	brokenPath.Checks = []doctor.CheckResult{
		{Name: "cloakbrowser_present", Status: doctor.StatusWarn, Detail: `configured browser.binary "/missing/cloak" could not be executed: no such file or directory`},
		{Name: "cdp_reachable", Status: doctor.StatusFail},
	}
	if hint := browserOverviewHint(brokenPath); !strings.Contains(hint, "Fix browser.binary") {
		t.Fatalf("broken binary hint = %q, want config repair guidance", hint)
	}
	if strings.Contains(browserOverviewHint(brokenPath), "was found") {
		t.Fatalf("broken binary hint = %q, must not claim browser was found", browserOverviewHint(brokenPath))
	}

	cdpFailure := base
	cdpFailure.Checks = []doctor.CheckResult{
		{Name: "cloakbrowser_present", Status: doctor.StatusPass, Detail: "/opt/cloak/chrome -> 130.0.0 (>= 120.0.0)"},
		{Name: "cdp_reachable", Status: doctor.StatusFail},
	}
	if hint := browserOverviewHint(cdpFailure); !strings.Contains(hint, "was found at /opt/cloak/chrome") {
		t.Fatalf("CDP failure hint = %q, want verified-binary repair guidance", hint)
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
		fc.Browser.BrowserBinary = missingBinary
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
