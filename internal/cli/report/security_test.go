package report

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
)

func TestAssessSecurityWarnings(t *testing.T) {
	t.Run("safe local defaults stay quiet", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
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

		warnings := AssessSecurityWarnings(cfg)
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %+v", warnings)
		}
	})

	t.Run("every loopback spelling stays quiet", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.2",
			Token:              "secret",
			AttachEnabled:      true,
			AttachAllowHosts:   []string{"127.0.0.2", "LOCALHOST", "::1"},
			AttachAllowSchemes: []string{"ws", "wss"},
			AllowedDomains:     []string{"127.0.0.1"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
				WrapContent: true,
			},
		}

		for _, warning := range AssessSecurityWarnings(cfg) {
			if warning.ID == "non_loopback_bind" || warning.ID == "attach_external_hosts" {
				t.Errorf("loopback-only config warned %s: %s", warning.ID, warning.Message)
			}
		}
	})

	t.Run("a localhost subdomain attach host is treated as external", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:             "127.0.0.1",
			AttachEnabled:    true,
			AttachAllowHosts: []string{"foo.localhost"},
		}

		for _, warning := range AssessSecurityWarnings(cfg) {
			if warning.ID == "attach_external_hosts" {
				return
			}
		}
		t.Error("foo.localhost can resolve off-host on pure-Go builds, yet no attach_external_hosts warning")
	})

	t.Run("website whitelist missing and other security gaps are flagged", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:             "0.0.0.0",
			Token:            "",
			AllowEvaluate:    true,
			AllowDownload:    true,
			AttachEnabled:    true,
			AttachAllowHosts: []string{"localhost", "chrome.internal"},
			IDPI: config.IDPIConfig{
				Enabled: true,
			},
		}

		warnings := AssessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		for _, warning := range warnings {
			ids[warning.ID] = true
		}

		for _, expected := range []string{
			"sensitive_endpoints_enabled",
			"api_auth_disabled",
			"sensitive_endpoints_without_auth",
			"non_loopback_bind",
			"idpi_whitelist_not_set",
			"idpi_warn_mode",
			"idpi_content_protection_disabled",
			"attach_external_hosts",
		} {
			if !ids[expected] {
				t.Fatalf("expected warning %q, got %+v", expected, warnings)
			}
		}

		for _, warning := range warnings {
			if warning.ID != "non_loopback_bind" {
				continue
			}
			if len(warning.Attrs) < 4 || warning.Attrs[3] != "non-loopback bind is a documented, non-default, security-reducing choice; keep a token set and review reverse proxy or port-publishing boundaries explicitly" {
				t.Fatalf("unexpected non_loopback_bind warning attrs: %+v", warning)
			}
		}
	})

	t.Run("wildcard whitelist is warned", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:           "127.0.0.1",
			Token:          "secret",
			AllowedDomains: []string{"*"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
			},
		}

		warnings := AssessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		for _, warning := range warnings {
			ids[warning.ID] = true
		}

		if !ids["idpi_whitelist_allows_all"] {
			t.Fatalf("expected wildcard whitelist warning, got %+v", warnings)
		}
	})

	t.Run("wildcard attach hosts are called out explicitly", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:             "127.0.0.1",
			Token:            "secret",
			AttachEnabled:    true,
			AttachAllowHosts: []string{"*"},
			AttachAllowSchemes: []string{
				"http",
				"https",
			},
		}

		warnings := AssessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		var wildcardWarning SecurityWarning
		for _, warning := range warnings {
			ids[warning.ID] = true
			if warning.ID == "attach_wildcard_hosts" {
				wildcardWarning = warning
			}
		}

		if !ids["attach_wildcard_hosts"] {
			t.Fatalf("expected wildcard attach warning, got %+v", warnings)
		}
		if ids["attach_external_hosts"] {
			t.Fatalf("expected wildcard attach warning to replace generic external-host warning, got %+v", warnings)
		}
		if wildcardWarning.Message != "attach allowHosts disables host allowlisting" {
			t.Fatalf("unexpected wildcard attach warning message: %+v", wildcardWarning)
		}
	})

	t.Run("disabled IDPI is warned", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
			AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
			AttachAllowSchemes: []string{"ws", "wss"},
		}

		warnings := AssessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		for _, warning := range warnings {
			ids[warning.ID] = true
		}

		if !ids["idpi_disabled"] {
			t.Fatalf("expected idpi_disabled warning, got %+v", warnings)
		}
	})
}

func TestAssessSecurityPosture(t *testing.T) {
	t.Run("fully locked local config scores all defaults", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
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

		posture := AssessSecurityPosture(cfg)
		if posture.Passed != posture.Total {
			t.Fatalf("expected all checks to pass, got %d/%d", posture.Passed, posture.Total)
		}
		if posture.Level != "LOCKED" {
			t.Fatalf("expected LOCKED posture, got %q", posture.Level)
		}
	})

	t.Run("wildcard attach scope is surfaced in posture detail", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
			AttachAllowHosts:   []string{"*"},
			AttachAllowSchemes: []string{"http", "https"},
			AllowedDomains:     []string{"example.com"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
				WrapContent: true,
			},
		}

		posture := AssessSecurityPosture(cfg)
		for _, check := range posture.Checks {
			if check.ID != "attach_local_only" {
				continue
			}
			if check.Detail != "wildcard (*)" {
				t.Fatalf("expected wildcard host scope detail, got %q", check.Detail)
			}
			return
		}

		t.Fatal("attach_local_only check not found")
	})

	t.Run("exposed config drops posture score", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:             "0.0.0.0",
			AllowEvaluate:    true,
			AllowDownload:    true,
			AttachEnabled:    true,
			AttachAllowHosts: []string{"chrome.internal"},
			IDPI: config.IDPIConfig{
				Enabled: true,
			},
		}

		posture := AssessSecurityPosture(cfg)
		if posture.Passed >= 3 {
			t.Fatalf("expected exposed posture below 3 passed checks, got %d/%d", posture.Passed, posture.Total)
		}
		if posture.Level != "EXPOSED" {
			t.Fatalf("expected EXPOSED posture, got %q", posture.Level)
		}
	})
}

func TestApplyRecommendedSecurityDefaults(t *testing.T) {
	allowEvaluate := true
	attachEnabled := true
	fc := &config.FileConfig{
		Server: config.ServerConfig{
			Port:  "9999",
			Bind:  "0.0.0.0",
			Token: "secret",
		},
		Security: config.SecurityConfig{
			AllowEvaluate: &allowEvaluate,
			Attach: config.AttachConfig{
				Enabled:    &attachEnabled,
				AllowHosts: []string{"chrome.internal"},
			},
			IDPI: &config.IDPIConfig{
				Enabled: false,
			},
		},
	}

	workflow.ApplyRecommendedSecurityDefaults(fc)

	if fc.Server.Port != "9999" {
		t.Fatalf("expected port to be preserved, got %q", fc.Server.Port)
	}
	if fc.Server.Token != "secret" {
		t.Fatalf("expected token to be preserved, got %q", fc.Server.Token)
	}
	if fc.Server.Bind != "127.0.0.1" {
		t.Fatalf("expected bind to reset to loopback, got %q", fc.Server.Bind)
	}
	if fc.Security.AllowEvaluate == nil || *fc.Security.AllowEvaluate {
		t.Fatalf("expected allowEvaluate to reset to false, got %+v", fc.Security.AllowEvaluate)
	}
	if fc.Security.Attach.Enabled == nil || *fc.Security.Attach.Enabled {
		t.Fatalf("expected attach.enabled to reset to false, got %+v", fc.Security.Attach.Enabled)
	}
	if !fc.Security.IDPI.Enabled {
		t.Fatalf("expected idpi to be enabled")
	}
}

func TestApplyRecommendedSecurityDefaults_LeavesAMissingTokenAlone(t *testing.T) {
	fc := &config.FileConfig{}

	workflow.ApplyRecommendedSecurityDefaults(fc)

	if fc.Server.Token != "" {
		t.Fatalf("applying security defaults generated a token %q; whether one may be added to an existing config is ProvisionFileToken's decision, and generating here bypasses the operator-config refusal", fc.Server.Token)
	}
}

func TestRestoreSecurityDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	allowEvaluate := true
	attachEnabled := true
	fc := &config.FileConfig{
		Server: config.ServerConfig{
			Port:  "9999",
			Bind:  "0.0.0.0",
			Token: "secret",
		},
		Security: config.SecurityConfig{
			AllowEvaluate: &allowEvaluate,
			Attach: config.AttachConfig{
				Enabled:    &attachEnabled,
				AllowHosts: []string{"chrome.internal"},
			},
			IDPI: &config.IDPIConfig{
				Enabled: false,
			},
		},
	}
	if err := config.SaveFileConfig(fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	gotPath, changed, err := workflow.RestoreSecurityDefaults()
	if err != nil {
		t.Fatalf("workflow.RestoreSecurityDefaults() error = %v", err)
	}
	if gotPath != configPath {
		t.Fatalf("workflow.RestoreSecurityDefaults() path = %q, want %q", gotPath, configPath)
	}
	if !changed {
		t.Fatalf("workflow.RestoreSecurityDefaults() changed = false, want true")
	}

	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	loaded := &config.FileConfig{}
	if err := config.PatchConfigJSON(loaded, string(saved)); err != nil {
		t.Fatalf("PatchConfigJSON() error = %v", err)
	}
	if loaded.Server.Port != "9999" {
		t.Fatalf("expected port to be preserved, got %q", loaded.Server.Port)
	}
	if loaded.Server.Token != "secret" {
		t.Fatalf("expected token to be preserved, got %q", loaded.Server.Token)
	}
	if loaded.Server.Bind != "127.0.0.1" {
		t.Fatalf("expected bind to be restored, got %q", loaded.Server.Bind)
	}
	if loaded.Security.Attach.Enabled == nil || *loaded.Security.Attach.Enabled {
		t.Fatalf("expected attach.enabled to be restored to false, got %+v", loaded.Security.Attach.Enabled)
	}
	if !loaded.Security.IDPI.Enabled {
		t.Fatalf("expected idpi to be enabled after restore")
	}
}

func TestRestoreSecurityDefaults_RefusesToProvisionIntoAnOperatorConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = workflow.RestoreSecurityDefaults()
	if !errors.Is(err, config.ErrOperatorConfigToken) {
		t.Fatalf("workflow.RestoreSecurityDefaults() error = %v, want the operator-config refusal; this path used to generate a credential into the operator's file and discard the error", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the operator's config changed on a refused restore:\nbefore: %s\nafter:  %s", before, after)
	}
}

func pathWithinDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func TestPathWithinDirRejectsSiblingWithSharedPrefix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")

	if !pathWithinDir(dir, filepath.Join(dir, "config.json")) {
		t.Fatal("path inside directory was rejected")
	}
	if pathWithinDir(dir, filepath.Join(dir+"-escaped", "config.json")) {
		t.Fatal("sibling path sharing directory prefix was accepted")
	}
}

func TestRestoreSecurityDefaults_TokenOnlyChangeOnTheDefaultPathIsSaved(t *testing.T) {
	tmpHome := t.TempDir()
	// This test clears PINCHTAB_CONFIG on purpose, so restoreSecurityDefaults
	// resolves the real default path. Redirecting that default therefore has to
	// work on every platform: HOME only moves it on darwin and linux, because
	// config.userConfigDir falls through to os.UserConfigDir elsewhere, which
	// reads %AppData% and never consults HOME. Both spellings are set because
	// os.Getenv is case-sensitive even where Windows is not.
	configHome := filepath.Join(tmpHome, "AppData", "Roaming")
	t.Setenv("HOME", tmpHome)
	t.Setenv("AppData", configHome)
	t.Setenv("APPDATA", configHome)
	t.Setenv("PINCHTAB_CONFIG", "")

	// Ask for the default path rather than assuming its shape. The hardcoded
	// unix layout was the defect: on Windows the fixture landed in
	// tmpHome\.pinchtab while restoreSecurityDefaults edited %AppData%.
	configPath := config.DefaultConfigPath()
	// A default that escaped tmpHome is the operator's own config, and this test
	// rewrites what it finds there. Fail loudly rather than proceed: before this
	// guard the escape was silent and the test still PASSED, because the token it
	// asserts on was already present in the file it should never have opened.
	if !pathWithinDir(tmpHome, configPath) {
		t.Fatalf("default config path %q escaped the test's temp dir %q; this test would be rewriting the real config", configPath, tmpHome)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}

	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	_, changed, err := workflow.RestoreSecurityDefaults()
	if err != nil {
		t.Fatalf("workflow.RestoreSecurityDefaults() error = %v", err)
	}
	if !changed {
		t.Fatalf("workflow.RestoreSecurityDefaults() changed = false, want true")
	}

	loaded, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if loaded.Server.Token == "" {
		t.Fatalf("expected generated token to be persisted on the default path")
	}
}
