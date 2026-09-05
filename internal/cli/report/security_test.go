package report

import (
	"bytes"
	"log/slog"
	"strings"
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
			AllowedDomains:     []string{"127.0.0.1", "localhost", "::1"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
				WrapContent: true,
			},
		}

		warnings := AssessSecurityWarnings(cfg)
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

		warnings := AssessSecurityWarnings(cfg)
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

		for _, warning := range warnings {
			if warning.ID != "non_loopback_bind" {
				continue
			}
			if len(warning.Attrs) < 4 || warning.Attrs[3] != "non-loopback bind is a documented, non-default, security-reducing choice; keep a token set and review reverse proxy or port-publishing boundaries explicitly" {
				t.Fatalf("unexpected non_loopback_bind warning attrs: %+v", warning)
			}
		}
	})

	t.Run("wildcard whitelist is warned", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:           "127.0.0.1",
			Token:          "secret",
			AllowedDomains: []string{"*"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
			},
		}

		warnings := AssessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		for _, warning := range warnings {
			ids[warning.ID] = true
		}

		if !ids["idpi_whitelist_allows_all"] {
			t.Fatalf("expected wildcard whitelist warning, got %+v", warnings)
		}
	})

	t.Run("wildcard attach hosts are called out explicitly", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:             "127.0.0.1",
			Token:            "secret",
			AttachEnabled:    true,
			AttachAllowHosts: []string{"*"},
			AttachAllowSchemes: []string{
				"http",
				"https",
			},
		}

		warnings := AssessSecurityWarnings(cfg)
		ids := make(map[string]bool, len(warnings))
		var wildcardWarning SecurityWarning
		for _, warning := range warnings {
			ids[warning.ID] = true
			if warning.ID == "attach_wildcard_hosts" {
				wildcardWarning = warning
			}
		}

		if !ids["attach_wildcard_hosts"] {
			t.Fatalf("expected wildcard attach warning, got %+v", warnings)
		}
		if ids["attach_external_hosts"] {
			t.Fatalf("expected wildcard attach warning to replace generic external-host warning, got %+v", warnings)
		}
		if wildcardWarning.Message != "attach allowHosts disables host allowlisting" {
			t.Fatalf("unexpected wildcard attach warning message: %+v", wildcardWarning)
		}
	})

	t.Run("disabled IDPI is warned", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
			AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
			AttachAllowSchemes: []string{"ws", "wss"},
		}

		warnings := AssessSecurityWarnings(cfg)
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
			AllowedDomains:     []string{"127.0.0.1", "localhost", "::1"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
				WrapContent: true,
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

	t.Run("wildcard attach scope is surfaced in posture detail", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			Bind:               "127.0.0.1",
			Token:              "secret",
			AttachAllowHosts:   []string{"*"},
			AttachAllowSchemes: []string{"http", "https"},
			AllowedDomains:     []string{"example.com"},
			IDPI: config.IDPIConfig{
				Enabled:     true,
				StrictMode:  true,
				ScanContent: true,
				WrapContent: true,
			},
		}

		posture := assessSecurityPosture(cfg)
		for _, check := range posture.Checks {
			if check.ID != "attach_local_only" {
				continue
			}
			if check.Detail != "wildcard (*)" {
				t.Fatalf("expected wildcard host scope detail, got %q", check.Detail)
			}
			return
		}

		t.Fatal("attach_local_only check not found")
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

func warningByID(warnings []SecurityWarning, id string) (SecurityWarning, bool) {
	for _, w := range warnings {
		if w.ID == id {
			return w, true
		}
	}
	return SecurityWarning{}, false
}

// The hop count is invisible at runtime and wrong in both directions silently, so
// trusting forwarding headers earns a warning that names the count the server
// actually uses and the rule it must satisfy. Off means the count is never read
// and there is nothing to confirm.
func TestTrustingForwardingHeadersNamesTheEffectiveHopCount(t *testing.T) {
	cases := []struct {
		name  string
		trust bool
		hops  int
		want  string
	}{
		{"off says nothing", false, 3, ""},
		{"on with the default reports one", true, 0, "1"},
		{"on with two reports two", true, 2, "2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{Bind: "127.0.0.1", Token: "secret", TrustProxyHeaders: tc.trust, TrustedProxyHops: tc.hops}
			warning, found := warningByID(AssessSecurityWarnings(cfg), "trusted_proxy_hops")
			if tc.want == "" {
				if found {
					t.Fatalf("headers are not trusted yet the hop count was warned about: %+v", warning)
				}
				return
			}
			if !found {
				t.Fatal("headers are trusted but no warning names the hop count")
			}
			if !strings.Contains(warning.Message, "read "+tc.want+" hop(s)") {
				t.Fatalf("message %q does not name the effective count %s", warning.Message, tc.want)
			}
			hint := warning.Hint()
			if !strings.Contains(hint, "confirm "+tc.want+" equals the number of proxies in front that APPEND") || !strings.Contains(hint, "a higher number lets a client choose") {
				t.Fatalf("hint %q does not state the rule an operator can act on", hint)
			}
			if strings.Contains(warning.Message+hint, "trustedProxyCIDRs") {
				t.Fatalf("the warning points at security.trustedProxyCIDRs, which governs navigation responses, not header trust: %q", hint)
			}
		})
	}
}

// The boot banner is LogSecurityWarnings and it must carry every warning the
// assessor returns, so a warning added later cannot reach the CLI and not the banner.
func TestTheBootBannerLogsEveryAssessedWarning(t *testing.T) {
	cfg := &config.RuntimeConfig{Bind: "0.0.0.0", Token: "", TrustProxyHeaders: true, TrustedProxyHops: 2, AllowEvaluate: true}
	assessed := AssessSecurityWarnings(cfg)
	if len(assessed) < 3 {
		t.Fatalf("only %d warnings assessed; too few to prove the banner carries them all", len(assessed))
	}

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	LogSecurityWarnings(cfg)

	logged := buf.String()
	for _, warning := range assessed {
		if !strings.Contains(logged, "warningId="+warning.ID) {
			t.Errorf("banner omits %s", warning.ID)
		}
	}
	if !strings.Contains(logged, "hops=2") {
		t.Errorf("banner does not carry the hop count: %s", logged)
	}
}
