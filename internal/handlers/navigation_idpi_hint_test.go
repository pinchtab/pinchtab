package handlers

import (
	"strings"
	"testing"
)

func TestIDPIAllowlistHint(t *testing.T) {
	hint := idpiAllowlistHint("https://example.com/some/path")
	if !strings.Contains(hint, "example.com") {
		t.Errorf("hint should name the blocked host; got %q", hint)
	}
	if !strings.Contains(hint, "security.allowedDomains") {
		t.Errorf("hint should name the allowlist config key; got %q", hint)
	}
	if !strings.Contains(hint, "server restart") {
		t.Errorf("hint should remind the user to restart; got %q", hint)
	}
	// Must be copy-paste-safe: no "…" placeholder (which users paste literally),
	// and it should preserve existing domains via a `config get` append.
	if strings.ContainsRune(hint, '…') {
		t.Errorf("hint must not contain the … placeholder; got %q", hint)
	}
	if !strings.Contains(hint, "config get security.allowedDomains") {
		t.Errorf("hint should append to existing domains via config get; got %q", hint)
	}

	// Hostless targets (e.g. about:blank) can't be allowlisted, so no hint.
	if got := idpiAllowlistHint("about:blank"); got != "" {
		t.Errorf("expected empty hint for hostless url; got %q", got)
	}
}

func TestIDPIScannerHint(t *testing.T) {
	hint := idpiScannerHint()
	if !strings.Contains(hint, "strictMode") {
		t.Errorf("scanner hint should point at strictMode; got %q", hint)
	}
	if !strings.Contains(hint, "server restart") {
		t.Errorf("scanner hint should remind the user to restart; got %q", hint)
	}
}
