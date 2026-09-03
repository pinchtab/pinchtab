package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func hostFormConfig(allowedDomains []string) *config.RuntimeConfig {
	return &config.RuntimeConfig{
		ActionTimeout:  time.Second,
		AllowedDomains: allowedDomains,
		IDPI:           config.IDPIConfig{Enabled: true, StrictMode: true},
	}
}

// The raw HTTP path is the only one that reaches validation without a scheme:
// the CLI normalizes through urls.Normalize and MCP through urls.Sanitize. So it
// is the one that has to normalize here, and the target it hands on must be the
// normalized one — a decision taken about "https://localhost:9222" and a
// navigation issued for "localhost:9222" are decisions about different strings.
func TestNavigateTargetsAreNormalizedBeforeTheSecurityDecision(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		wantURL string
	}{
		{"loopback with the cdp port", "localhost:9222", "https://localhost:9222"},
		{"loopback bare", "localhost", "https://localhost"},
		{"loopback with path", "localhost/status", "https://localhost/status"},
		{"loopback literal ip with port", "127.0.0.1:9222", "https://127.0.0.1:9222"},
		{"already scheme-ful is untouched", "http://localhost:9222/x", "http://localhost:9222/x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(&policyMockBridge{}, hostFormConfig([]string{"localhost", "127.0.0.1"}), nil, nil, nil)
			w := httptest.NewRecorder()

			targets, ok := h.validateNavigateTargets(w, httptest.NewRequest("GET", "/navigate", nil), "", tc.rawURL, h.Config)

			if !ok {
				t.Fatalf("a listed host was refused: %d %s", w.Code, w.Body.String())
			}
			if targets.url != tc.wantURL {
				t.Errorf("target url = %q, want %q; the caller navigates to this, not to what it decoded", targets.url, tc.wantURL)
			}
		})
	}
}

// The allowlist used to refuse every scheme-less "host:port" — listed or not,
// including the canonical CDP spelling — because url.Parse read the leading label
// as a scheme and the empty host lost. The refusal even named the wrong cause:
// "URL has no domain component".
func TestAListedHostWrittenWithAPortIsNotRefusedByTheAllowlist(t *testing.T) {
	h := New(&policyMockBridge{}, hostFormConfig([]string{"localhost"}), nil, nil, nil)
	w := httptest.NewRecorder()

	if _, ok := h.validateNavigateTargets(w, httptest.NewRequest("GET", "/navigate", nil), "", "localhost:9222", h.Config); !ok {
		t.Fatalf("localhost:9222 refused under an allowlist that lists localhost: %d %s", w.Code, w.Body.String())
	}
}

// A host outside the list is still refused, and the refusal names the allowlist
// rather than reporting an SSRF or resolution failure — the misdiagnosis this
// card started from, where the operator was told the target was a private-IP
// threat when the real answer was that it was not on the list.
func TestAnUnlistedSingleLabelHostIsRefusedNamingTheAllowlist(t *testing.T) {
	h := New(&policyMockBridge{}, hostFormConfig([]string{"example.com"}), nil, nil, nil)
	w := httptest.NewRecorder()

	if _, ok := h.validateNavigateTargets(w, httptest.NewRequest("GET", "/navigate", nil), "", "intranet", h.Config); ok {
		t.Fatal("an unlisted host was accepted under an active allowlist")
	}

	resp := decodeBlocked(t, w)
	if !strings.Contains(resp.Error, "allowlist") {
		t.Errorf("refusal = %q; it must name the list that refused, not a network cause", resp.Error)
	}
	if got := detailString(t, resp.Details, "hint"); !strings.Contains(got, "security.allowedDomains") {
		t.Errorf("hint = %q; it does not name the setting that refused", got)
	}
	for _, misdiagnosis := range []string{"private", "internal IP", "resolve", "no domain component"} {
		if strings.Contains(resp.Error, misdiagnosis) {
			t.Errorf("refusal = %q; it reports %q, which is not why this was refused", resp.Error, misdiagnosis)
		}
	}
	if got := detailString(t, resp.Details, "remedy"); !strings.Contains(got, "intranet") {
		t.Errorf("remedy = %q; it does not name the host the caller must list", got)
	}
}
