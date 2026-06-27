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

	// Hostless targets (e.g. about:blank) can't be allowlisted, so no hint.
	if got := idpiAllowlistHint("about:blank"); got != "" {
		t.Errorf("expected empty hint for hostless url; got %q", got)
	}
}
