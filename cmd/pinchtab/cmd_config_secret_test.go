package main

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// `pinchtab config get <path>` prints the value, and `config set` echoes it
// back, so every config path holding secret material must be masked. The
// schema's secret-bearing fields are capsolverKey, twoCaptchaKey,
// stateEncryptionKey, password, token and the credentials subtree.
//
// The rule is config.IsSensitiveConfigPath, the one path-name vocabulary, which
// the preset change report shares; this test pins that these two commands still
// consult it for every secret field, because a surface that publishes config
// values by path and skips it prints the value. security.stateEncryptionKey is
// only readable through the editor at all because the section-leaf walk in
// internal/config required every declared leaf to resolve, so the vocabulary is
// what keeps that reachability from printing a secret.
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
		if !config.IsSensitiveConfigPath(path) {
			t.Errorf("config.IsSensitiveConfigPath(%q) = false, want true — value would print in clear", path)
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
		if config.IsSensitiveConfigPath(path) {
			t.Errorf("config.IsSensitiveConfigPath(%q) = true, want false — ordinary setting would be masked", path)
		}
	}
}
