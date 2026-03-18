package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestLoadConfigWithMandatoryToken_GeneratesAndPersistsToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	cfg, err := loadConfigWithMandatoryToken()
	if err != nil {
		t.Fatalf("loadConfigWithMandatoryToken() error = %v", err)
	}
	if cfg.Token == "" {
		t.Fatal("expected generated runtime token")
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Server.Token == "" {
		t.Fatal("expected generated token to be persisted")
	}
	if saved.Server.Token != cfg.Token {
		t.Fatalf("saved token = %q, runtime token = %q", saved.Server.Token, cfg.Token)
	}
}

func TestLoadConfigWithMandatoryToken_UsesEnvTokenWithoutPersisting(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "env-token")

	cfg, err := loadConfigWithMandatoryToken()
	if err != nil {
		t.Fatalf("loadConfigWithMandatoryToken() error = %v", err)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("runtime token = %q, want env-token", cfg.Token)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected config file to remain absent, stat err = %v", err)
	}
}

func TestLoadConfigWithMandatoryToken_PreservesExistingToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	fc := config.DefaultFileConfig()
	fc.Server.Token = "existing-token"
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	cfg, err := loadConfigWithMandatoryToken()
	if err != nil {
		t.Fatalf("loadConfigWithMandatoryToken() error = %v", err)
	}
	if cfg.Token != "existing-token" {
		t.Fatalf("runtime token = %q, want existing-token", cfg.Token)
	}
}

func TestLoadConfigWithMandatoryToken_ExplicitConfigRequiresToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	fc := config.DefaultFileConfig()
	fc.ConfigVersion = config.CurrentConfigVersion
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	_, err := loadConfigWithMandatoryToken()
	if err == nil {
		t.Fatal("expected explicit config without token to fail")
	}
	if !strings.Contains(err.Error(), "server token is required") {
		t.Fatalf("error = %q, want explicit token requirement", err)
	}
}
