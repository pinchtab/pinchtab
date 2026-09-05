package workflow

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

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

	ApplyRecommendedSecurityDefaults(fc)

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

	ApplyRecommendedSecurityDefaults(fc)

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

	gotPath, changed, err := RestoreSecurityDefaults()
	if err != nil {
		t.Fatalf("RestoreSecurityDefaults() error = %v", err)
	}
	if gotPath != configPath {
		t.Fatalf("RestoreSecurityDefaults() path = %q, want %q", gotPath, configPath)
	}
	if !changed {
		t.Fatalf("RestoreSecurityDefaults() changed = false, want true")
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

	_, _, err = RestoreSecurityDefaults()
	if !errors.Is(err, config.ErrOperatorConfigToken) {
		t.Fatalf("RestoreSecurityDefaults() error = %v, want the operator-config refusal; this path used to generate a credential into the operator's file and discard the error", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the operator's config changed on a refused restore:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRestoreSecurityDefaults_TokenOnlyChangeOnTheDefaultPathIsSaved(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PINCHTAB_CONFIG", "")
	configPath := filepath.Join(tmpHome, ".pinchtab", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}

	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	_, changed, err := RestoreSecurityDefaults()
	if err != nil {
		t.Fatalf("RestoreSecurityDefaults() error = %v", err)
	}
	if !changed {
		t.Fatalf("RestoreSecurityDefaults() changed = false, want true")
	}

	loaded, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if loaded.Server.Token == "" {
		t.Fatalf("expected generated token to be persisted on the default path")
	}
}
