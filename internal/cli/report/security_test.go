package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestAssessSecurityWarnings(t *testing.T) {
	t.Run("safe local defaults stay quiet", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
			AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
			AttachAllowSchemes: []string{"ws", "wss"},
			IDPI: config.IDPIConfig{
				Enabled:        true,
				AllowedDomains: []string{"127.0.0.1", "localhost", "::1"},
				StrictMode:     true,
				ScanContent:    true,
				WrapContent:    true,
			},
		}

		warnings := assessSecurityWarnings(cfg)
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %+v", warnings)
		}
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

		warnings := assessSecurityWarnings(cfg)
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
	})

	t.Run("wildcard whitelist is warned", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:  "127.0.0.1",
			Token: "secret",
			IDPI: config.IDPIConfig{
				Enabled:        true,
				AllowedDomains: []string{"*"},
				StrictMode:     true,
				ScanContent:    true,
			},
		}

		warnings := assessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		for _, warning := range warnings {
			ids[warning.ID] = true
		}

		if !ids["idpi_whitelist_allows_all"] {
			t.Fatalf("expected wildcard whitelist warning, got %+v", warnings)
		}
	})

	t.Run("disabled IDPI is warned", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
			AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
			AttachAllowSchemes: []string{"ws", "wss"},
		}

		warnings := assessSecurityWarnings(cfg)
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
			IDPI: config.IDPIConfig{
				Enabled:        true,
				AllowedDomains: []string{"127.0.0.1", "localhost", "::1"},
				StrictMode:     true,
				ScanContent:    true,
				WrapContent:    true,
			},
		}

		posture := assessSecurityPosture(cfg)
		if posture.Passed != posture.Total {
			t.Fatalf("expected all checks to pass, got %d/%d", posture.Passed, posture.Total)
		}
		if posture.Level != "LOCKED" {
			t.Fatalf("expected LOCKED posture, got %q", posture.Level)
		}
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

		posture := assessSecurityPosture(cfg)
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
			IDPI: config.IDPIConfig{
				Enabled: false,
			},
		},
	}

	applyRecommendedSecurityDefaults(fc)

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

func TestApplyRecommendedSecurityDefaults_GeneratesTokenWhenMissing(t *testing.T) {
	fc := &config.FileConfig{}

	applyRecommendedSecurityDefaults(fc)

	if fc.Server.Token == "" {
		t.Fatalf("expected generated token, got empty")
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
			IDPI: config.IDPIConfig{
				Enabled: false,
			},
		},
	}
	if err := config.SaveFileConfig(fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	gotPath, changed, err := restoreSecurityDefaults()
	if err != nil {
		t.Fatalf("restoreSecurityDefaults() error = %v", err)
	}
	if gotPath != configPath {
		t.Fatalf("restoreSecurityDefaults() path = %q, want %q", gotPath, configPath)
	}
	if !changed {
		t.Fatalf("restoreSecurityDefaults() changed = false, want true")
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

func TestRestoreSecurityDefaults_TokenOnlyChangeIsSaved(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	_, changed, err := restoreSecurityDefaults()
	if err != nil {
		t.Fatalf("restoreSecurityDefaults() error = %v", err)
	}
	if !changed {
		t.Fatalf("restoreSecurityDefaults() changed = false, want true")
	}

	loaded, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if loaded.Server.Token == "" {
		t.Fatalf("expected generated token to be persisted")
	}
}
