package security

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestHostAllowed_EmptyAllowlistMeansNoEnforcement(t *testing.T) {
	if !HostAllowed("https://anything.test/", nil) {
		t.Error("nil allowlist should mean no enforcement → allowed")
	}
	if !HostAllowed("https://anything.test/", []string{}) {
		t.Error("empty allowlist should mean no enforcement → allowed")
	}
}

func TestHostAllowed_ExactMatch(t *testing.T) {
	allow := []string{"api.example.com"}
	if !HostAllowed("https://api.example.com/users", allow) {
		t.Error("exact match should be allowed")
	}
	if HostAllowed("https://other.example.com/users", allow) {
		t.Error("non-matching host should be blocked")
	}
}

func TestHostAllowed_WildcardSubdomain(t *testing.T) {
	allow := []string{"*.example.com"}
	if !HostAllowed("https://api.example.com/", allow) {
		t.Error("*.example.com should match api.example.com")
	}
	if HostAllowed("https://example.com/", allow) {
		t.Error("*.example.com should NOT match example.com (apex)")
	}
}

func TestHostAllowed_GlobalWildcard(t *testing.T) {
	if !HostAllowed("https://anything.test/", []string{"*"}) {
		t.Error("'*' should allow any host")
	}
}

func TestHostAllowed_AboutBlankBypassesCheck(t *testing.T) {
	allow := []string{"api.example.com"}
	if !HostAllowed("about:blank", allow) {
		t.Error("about:blank should bypass the host check (no host to verify)")
	}
}

func TestHostAllowed_PortStripped(t *testing.T) {
	allow := []string{"localhost"}
	if !HostAllowed("http://localhost:8080/page", allow) {
		t.Error("port should be stripped before host comparison")
	}
}

func TestHostAllowed_CaseInsensitive(t *testing.T) {
	allow := []string{"API.example.com"}
	if !HostAllowed("https://api.EXAMPLE.com/", allow) {
		t.Error("host comparison should be case-insensitive")
	}
}

func TestExtractHost_BareHostname(t *testing.T) {
	if got := ExtractHost("example.com/path"); got != "example.com" {
		t.Errorf("ExtractHost('example.com/path') = %q, want 'example.com'", got)
	}
}

func TestExtractHost_FullURL(t *testing.T) {
	if got := ExtractHost("https://example.com:8080/path?q=1"); got != "example.com" {
		t.Errorf("ExtractHost full URL = %q, want 'example.com'", got)
	}
}

func TestExtractHost_NoHost(t *testing.T) {
	if got := ExtractHost("about:blank"); got != "" {
		t.Errorf("ExtractHost('about:blank') = %q, want ''", got)
	}
}

// A fully-qualified name carrying the silent DNS root label is the same host:
// the browser resolves and fetches "example.com." exactly as "example.com".
// The allowlist is read in both directions, so a host that stops matching is
// wrong twice over — a refusal on a target the operator listed, and, where the
// answer is read inverted (bridge response forgery is BLOCKED on listed hosts),
// permission to forge responses on the sensitive origin the list marks.
func TestHostAllowed_TrailingRootLabelIsTheSameHost(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		allowed []string
	}{
		{"exact", "https://example.com./path", []string{"example.com"}},
		{"exact with port", "https://example.com.:8443/path", []string{"example.com"}},
		{"wildcard subdomain", "https://api.example.com./v1", []string{"*.example.com"}},
		{"bare hostname", "example.com./path", []string{"example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !HostAllowed(tc.rawURL, tc.allowed) {
				t.Errorf("HostAllowed(%q, %v) = false; the root label names the same host", tc.rawURL, tc.allowed)
			}
		})
	}
}

// The trim must not reach past the root label into the name itself, and it must
// not turn a non-match into a match.
func TestHostAllowed_TrailingRootLabelDoesNotWidenTheList(t *testing.T) {
	if HostAllowed("https://evil.com./path", []string{"example.com"}) {
		t.Error("an unlisted host matched the allowlist")
	}
	if got := ExtractHost("https://example.com../path"); got != "example.com." {
		t.Errorf("ExtractHost trimmed more than the single root label: %q", got)
	}
}

// The forms a host is written in, driven through the ALLOWLIST DECISION rather
// than the parse, because the decision is what the operator gets. Three
// extractors used to answer this question and they disagreed on the first two
// groups below: a single-label intranet name was a host to one and nothing to
// another, and every scheme-less "host:port" was nothing to all of them, so a
// listed host written with a port was refused by the allowlist itself.
func TestHostAllowedAcrossTheFormsAHostIsWrittenIn(t *testing.T) {
	allowed := []string{"intranet", "example.com", "localhost", "192.168.1.5", "*.corp.example.com"}

	cases := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{"single-label bare host", "intranet", true},
		{"single-label with path", "intranet/wiki", true},
		{"single-label with query", "intranet?q=1", true},
		{"single-label with port", "intranet:8080", true},
		{"single-label unlisted", "wiki", false},

		{"bare host with port", "example.com:8080", true},
		{"bare host with port and path", "example.com:8080/x", true},
		{"loopback with the cdp port", "localhost:9222", true},
		{"literal ip with port", "192.168.1.5:9000", true},

		{"scheme-ful", "https://example.com/path", true},
		{"scheme-ful with port", "https://example.com:8443/path?q=1", true},
		{"scheme-ful uppercase", "HTTPS://EXAMPLE.COM", true},
		{"bare host", "example.com", true},
		{"literal ip", "192.168.1.5", true},
		{"localhost", "localhost", true},

		{"trailing root label", "example.com./path", true},
		{"trailing root label with port", "example.com.:8443", true},
		{"wildcard subdomain", "https://api.corp.example.com/v1", true},
		{"wildcard subdomain bare with port", "api.corp.example.com:8443", true},

		// The protocol-relative form and the slash runs a browser skips after a
		// special scheme. These read as hostless to net/url, so this primitive
		// answered "" for them and refused a listed host — and, read INVERTED by
		// the response-forgery rule, a listed host that stops matching starts
		// PERMITTING forgery on exactly the sensitive origin the list marks.
		{"protocol-relative", "//example.com/x", true},
		{"protocol-relative with port", "//example.com:8443/x", true},
		{"protocol-relative unlisted", "//evil.com/x", false},
		{"single leading slash", "/example.com/x", true},
		{"special scheme, no slashes", "https:example.com/x", true},
		{"special scheme, one slash", "https:/example.com/x", true},
		{"special scheme, four slashes", "https:////example.com/x", true},
		{"special scheme, backslashes", `https:\\example.com\x`, true},
		{"special scheme, backslashes unlisted", `https:\\evil.com\x`, false},

		{"unlisted host", "https://evil.com", false},
		{"unlisted host with port", "evil.com:8080", false},
		{"unlisted subdomain of a listed host", "https://evil.example.com.attacker.test", false},
		{"about:blank has no host to verify", "about:blank", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HostAllowed(tc.rawURL, allowed); got != tc.want {
				t.Errorf("HostAllowed(%q, %v) = %v, want %v (host extracted: %q)",
					tc.rawURL, allowed, got, tc.want, ExtractHost(tc.rawURL))
			}
		})
	}
}

// The case both surviving extractors used to get wrong, kept as its own test
// because it fails for EVERY host rather than for an unlucky one: url.Parse reads
// "localhost" in "localhost:9222" as a scheme, so the host came back empty and
// HostAllowed refused on the empty host — the canonical CDP spelling refused by
// the allowlist that lists it.
func TestAListedHostWrittenWithAPortIsAllowed(t *testing.T) {
	for _, rawURL := range []string{"localhost:9222", "example.com:8080", "192.168.1.5:9000", "intranet:8080"} {
		host := ExtractHost(rawURL)
		if host == "" {
			t.Errorf("ExtractHost(%q) = %q; an empty host is refused by the allowlist whatever it lists", rawURL, host)
			continue
		}
		if !HostAllowed(rawURL, []string{host}) {
			t.Errorf("HostAllowed(%q, [%q]) = false; the host it names is the one listed", rawURL, host)
		}
	}
}

// The rule the card is named for, kept enforceable: one extractor answers "which
// host is this security decision about". Three used to, they disagreed on exactly
// the forms an intranet host is written in, and the direction of the disagreement
// was luck rather than design. A fourth copy — or a resurrection of one of the
// two deleted — reds here by file and line rather than by whichever consumer
// happens to read the losing answer.
func TestOnePackageOwnsTheSecurityHostExtractor(t *testing.T) {
	files := srccensus.Tree(t, "../..", 200)

	var declarations []string
	for _, file := range files {
		if strings.Contains(file.Text, "func ExtractHost(") {
			declarations = append(declarations, file.Name)
		}
	}

	if len(declarations) != 1 {
		t.Fatalf("ExtractHost is declared in %d files, want exactly 1:\n  %s",
			len(declarations), strings.Join(declarations, "\n  "))
	}
	if filepath.Base(filepath.Dir(declarations[0])) != "security" {
		t.Errorf("the one extractor lives in %s; internal/security owns this primitive because every consumer already imports it and it imports none of them", declarations[0])
	}
}

// The direction this primitive is read in by internal/bridge's response-forgery
// rule, which inverts it: forgery is BLOCKED on listed hosts. A spelling that stops
// matching the allowlist therefore does not merely refuse a navigation — it PERMITS
// response forgery on the origin the operator marked as sensitive. That is the half
// of the widening the navigation tests cannot see, so it is asserted here.
func TestASpellingThatStopsMatchingWouldPermitForgeryOnAListedHost(t *testing.T) {
	allowed := []string{"bank.example.com"}

	for _, rawURL := range []string{
		"https://bank.example.com/transfer",
		"//bank.example.com/transfer",
		"//bank.example.com:8443/transfer",
		"/bank.example.com/transfer",
		"https:bank.example.com/transfer",
		"https:/bank.example.com/transfer",
		"https:////bank.example.com/transfer",
		`https:\\bank.example.com\transfer`,
	} {
		t.Run(rawURL, func(t *testing.T) {
			if !HostAllowed(rawURL, allowed) {
				t.Errorf("HostAllowed(%q) = false (host extracted: %q) — the forgery rule reads this inverted, so a listed host that stops matching starts permitting forged responses on the sensitive origin the list marks",
					rawURL, ExtractHost(rawURL))
			}
		})
	}
}
