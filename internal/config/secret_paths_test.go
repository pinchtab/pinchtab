package config

import "testing"

// The properties the comment justifies, each its own row: suffix on the leaf so
// the *Key fields match, case-insensitive, credentials at any depth, and the
// suffix confined to the leaf so a user-named intermediate segment masks nothing.
func TestIsSensitiveConfigPathVocabulary(t *testing.T) {
	cases := map[string]bool{
		"server.token":                           true,
		"server.apiToken":                        true,
		"Server.Password":                        true,
		"security.stateEncryptionKey":            true,
		"autoSolver.external.capsolverKey":       true,
		"autoSolver.external.twoCaptchaKey":      true,
		"browser.proxy.password":                 true,
		"browser.targets.work.proxy.password":    true,
		"autoSolver.credentials.login.username":  true,
		"autoSolver.credentials.signup.password": true,
		"server.bind":                            false,
		"security.idpi.enabled":                  false,
		"browser.targets.monkey.headless":        false,
		"browser.targets.secretary.url":          false,
		"server.tokenRotationDays":               false,
		"security.allowedDomains":                false,
	}
	for path, want := range cases {
		if got := IsSensitiveConfigPath(path); got != want {
			t.Errorf("IsSensitiveConfigPath(%q) = %v, want %v", path, got, want)
		}
	}
}
