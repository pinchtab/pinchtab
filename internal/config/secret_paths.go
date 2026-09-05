package config

import "strings"

// IsSensitiveConfigPath reports whether a dotted config path points at secret
// material, so any surface that prints a value by path masks it first. It is
// the one path-name vocabulary; the dashboard's typed-field redaction is a
// different mechanism with its own guard.
//
// The leaf is matched by suffix rather than exact name: the schema's secrets are
// stateEncryptionKey, capsolverKey and twoCaptchaKey alongside token/password,
// and an exact-match list silently missed every one of the *Key fields. Suffix
// matching is confined to the leaf because intermediate segments are
// user-chosen names (a browser target called "monkey" must not mask the whole
// subtree). The credentials subtree is secret at every depth, so it matches
// wherever it appears.
func IsSensitiveConfigPath(path string) bool {
	segments := strings.Split(strings.ToLower(strings.TrimSpace(path)), ".")
	for _, segment := range segments {
		if segment == "credentials" {
			return true
		}
	}
	last := segments[len(segments)-1]
	for _, suffix := range []string{"token", "password", "secret", "key", "passphrase"} {
		if strings.HasSuffix(last, suffix) {
			return true
		}
	}
	return false
}
