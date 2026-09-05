package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
)

func TestHandleSecurityCommandDefaultConfigSkipsEmptySections(t *testing.T) {
	cfg := testRuntimeConfig()

	output := captureStdout(t, func() {
		printSecurityOverview(cfg, nil, nil)
	})

	required := []string{
		"Security",
		"All recommended security defaults are active.",
	}
	for _, needle := range required {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected output to contain %q\n%s", needle, output)
		}
	}

	unwanted := []string{
		"Security posture",
		"Warnings",
		"Recommended security defaults",
		"Recommended defaults",
		"Restore recommended security defaults in config?",
		"Interactive restore skipped because stdin/stdout is not a terminal.",
	}
	for _, needle := range unwanted {
		if strings.Contains(output, needle) {
			t.Fatalf("expected output to skip %q\n%s", needle, output)
		}
	}
}

func TestPrintSecurityOverviewDoesNotCallAuthDisabledSafe(t *testing.T) {
	cfg := testRuntimeConfig()
	cfg.Token = ""

	output := captureStdout(t, func() {
		printSecurityOverview(cfg, nil, nil)
	})

	if strings.Contains(output, "All recommended security defaults are active.") {
		t.Fatalf("auth-disabled config should not be reported as fully recommended\n%s", output)
	}
	if !strings.Contains(output, "security warning") {
		t.Fatalf("expected security warning summary for auth-disabled config\n%s", output)
	}
}

func TestApplySecurityDownPrintsExplicitRiskFraming(t *testing.T) {
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

	output := captureStdout(t, func() {
		result, err := applySecurityDown(false)
		if err != nil {
			t.Fatalf("applySecurityDown() error = %v", err)
		}
		if !result.Written {
			t.Fatal("expected applySecurityDown() to change config")
		}
	})

	for _, needle := range []string{
		"Guards down preset applied",
		"This is a documented, non-default, security-reducing preset.",
		"sensitive endpoints and attach are enabled, and IDPI protections are disabled.",
		"Attach host allowlisting remains local-only.",
		"Changing server.bind away from 127.0.0.1 later is also an additional explicit weakening",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected output to contain %q\n%s", needle, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	return captureStream(t, &os.Stdout, fn)
}

func captureStderr(t *testing.T, fn func()) string {
	return captureStream(t, &os.Stderr, fn)
}

func captureStream(t *testing.T, target **os.File, fn func()) string {
	t.Helper()

	orig := *target
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	*target = w

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, r)
		_ = r.Close()
		done <- err
	}()

	defer func() {
		*target = orig
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer error = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("io.Copy() error = %v", err)
	}
	return buf.String()
}

func testRuntimeConfig() *config.RuntimeConfig {
	return &config.RuntimeConfig{
		Bind:               "127.0.0.1",
		Token:              "abcd1234efgh5678",
		AllowEvaluate:      false,
		AllowMacro:         false,
		AllowScreencast:    false,
		AllowDownload:      false,
		AllowCookies:       false,
		AllowUpload:        false,
		AttachEnabled:      false,
		AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
		AttachAllowSchemes: []string{"ws", "wss"},
		AllowedDomains:     []string{"127.0.0.1", "localhost", "::1"},
		IDPI: config.IDPIConfig{
			Enabled:     true,
			StrictMode:  true,
			ScanContent: true,
			WrapContent: true,
		},
	}
}

func TestPrintSecurityOverviewNamesWarningsSecurityUpCannotFix(t *testing.T) {
	cfg := testRuntimeConfig()
	cfg.AttachAllowSchemes = []string{"ws", "wss", "http", "https"}
	cfg.AllowedDomains = nil

	output := captureStdout(t, func() {
		printSecurityOverview(cfg, nil, nil)
	})

	if !strings.Contains(output, "website whitelist is not set for IDPI") || !strings.Contains(output, "configure allowedDomains") {
		t.Fatalf("expected the whitelist warning and its hint\n%s", output)
	}
	if strings.Contains(output, "differ from recommended defaults") || strings.Contains(output, "warning(s) detected — pinchtab security up") {
		t.Fatalf("security up cannot set a whitelist, so the overview must not send the operator there\n%s", output)
	}
}

// The overview advertises the preset's own dry run, so its count and the count
// security up then reports are one number; a config missing the recommended
// defaults is where two hand-kept lists used to disagree by one.
func TestOverviewCountIsThePresetChangeCount(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := os.WriteFile(configPath, []byte(`{"server":{"bind":"0.0.0.0","token":"secret"},"security":{"allowEvaluate":true}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changes, err := recommendedChanges()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) < 4 {
		t.Fatalf("the relaxed fixture yields only %d changes; too few to tell the lists apart", len(changes))
	}
	output := captureStdout(t, func() {
		printSecurityOverview(config.Load(), changes, nil)
	})
	want := fmt.Sprintf("%d config setting(s) differ from recommended defaults", len(changes))
	if !strings.Contains(output, want) {
		t.Fatalf("overview does not advertise %q\n%s", want, output)
	}
	for _, change := range changes {
		if !strings.Contains(output, change.Path+":") {
			t.Errorf("overview omits %s, which security up would write", change.Path)
		}
	}
	preview, err := workflow.RestoreSecurityDefaults(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != len(changes) {
		t.Fatalf("overview counts %d, the preset would write %d", len(changes), len(preview.Changes))
	}
}

// The overview is the second surface for security warnings, driven off the same
// assessor as the boot banner, so every warning the assessor returns must print —
// whatever the recommended list holds, and even when computing it failed. A live
// statement about the running server outranks an offer to reconcile settings, so
// the warnings print first and the all-clear line never stands above one.
func TestOverviewPrintsEveryAssessedWarning(t *testing.T) {
	cfg := testRuntimeConfig()
	cfg.Token = ""
	cfg.TrustProxyHeaders = true
	cfg.TrustedProxyHops = 2
	assessed := cli.AssessSecurityWarnings(cfg)
	if len(assessed) < 2 {
		t.Fatalf("only %d warnings assessed; too few to prove the overview prints them all", len(assessed))
	}
	oneChange := []workflow.SettingChange{{Path: "server.token", Old: "", New: "<generated>"}}
	cases := []struct {
		name    string
		changes []workflow.SettingChange
		err     error
	}{
		{"nothing differs", nil, nil},
		{"a recommended default differs", oneChange, nil},
		{"the recommended computation failed", nil, errors.New("config unreadable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				printSecurityOverview(cfg, tc.changes, tc.err)
			})
			for _, warning := range assessed {
				if !strings.Contains(output, warning.Message) {
					t.Errorf("overview omits %q\n%s", warning.Message, output)
				}
				if hint := warning.Hint(); hint != "" && !strings.Contains(output, hint) {
					t.Errorf("overview omits the hint for %s\n%s", warning.ID, output)
				}
			}
			if strings.Contains(output, "All recommended security defaults are active.") {
				t.Errorf("the all-clear line printed while warnings stand\n%s", output)
			}
			if len(tc.changes) > 0 {
				warned := strings.Index(output, "security warning(s) detected")
				differs := strings.Index(output, "differ from recommended defaults")
				if differs < 0 || warned < 0 || warned > differs {
					t.Errorf("warnings must print before the recommended list (warnings at %d, list at %d)\n%s", warned, differs, output)
				}
			}
			if tc.err != nil && !strings.Contains(output, "could not compute") {
				t.Errorf("a failed recommended computation was not reported\n%s", output)
			}
		})
	}
}
