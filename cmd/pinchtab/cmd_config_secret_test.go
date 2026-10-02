package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// `pinchtab config get <path>` prints the value, and `config set` echoes it
// back, so every config path holding secret material must be masked. The
// schema's secret-bearing fields are capsolverKey, twoCaptchaKey,
// stateEncryptionKey, password, token and the credentials subtree.
//
// This is where the masking rule lives, and it is a DISPLAY property of these
// two commands rather than of the field: a surface that publishes config values
// some other way inherits none of it. security.stateEncryptionKey is only
// readable through the editor at all because the section-leaf walk in
// internal/config required every declared leaf to resolve, so this list is what
// keeps that reachability from printing a secret.
func TestIsSensitiveConfigPathCoversEverySecretField(t *testing.T) {
	sensitive := []string{
		"server.token",
		"browser.proxy.password",
		"security.stateEncryptionKey",
		"autoSolver.external.capsolverKey",
		"autoSolver.external.twoCaptchaKey",
		"autoSolver.credentials.login.password",
		"autoSolver.credentials.signup.email",
		"browser.targets.default.proxy.password",
	}
	for _, path := range sensitive {
		if !isSensitiveConfigPath(path) {
			t.Errorf("isSensitiveConfigPath(%q) = false, want true — value would print in clear", path)
		}
	}
}

// Masking must stay narrow enough that ordinary settings remain readable.
func TestIsSensitiveConfigPathLeavesOrdinarySettingsVisible(t *testing.T) {
	ordinary := []string{
		"server.port",
		"server.bind",
		"browser.targets.default.provider",
		"instanceDefaults.mode",
		"scheduler.maxInflight",
		"security.allowEvaluate",
		"observability.activity.retentionDays",
	}
	for _, path := range ordinary {
		if isSensitiveConfigPath(path) {
			t.Errorf("isSensitiveConfigPath(%q) = true, want false — ordinary setting would be masked", path)
		}
	}
}

func TestConfigGetNotesWhenValueIsMasked(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	fc := config.DefaultFileConfig()
	fc.Server.Token = "test-token-for-masked-get-0000000"
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() { handleConfigGet("server.token") })
	})

	if want := config.MaskToken(fc.Server.Token) + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if strings.Count(stderr, "Note:") != 1 || !strings.Contains(stderr, "pinchtab config token --stdout") {
		t.Errorf("stderr = %q, want one Note naming pinchtab config token --stdout", stderr)
	}
}

func TestConfigGetAddsNothingToStderrForOrdinaryPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	fc := config.DefaultFileConfig()
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() { handleConfigGet("server.port") })
	})

	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}
