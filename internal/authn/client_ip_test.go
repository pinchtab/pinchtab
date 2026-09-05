package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestResolveClientIPIgnoresForwardingHeadersWhenProxyIsUntrusted(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:43123"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	req.Header.Set("Forwarded", "for=203.0.113.7")

	if got := ResolveClientIP(req, false, 1); got != "198.51.100.10" {
		t.Fatalf("ResolveClientIP(untrusted) = %q, want the peer address", got)
	}
}

// A stripping, rebuilding proxy leaves exactly one element, so the address it
// wrote is both the client-most and the first from the right: every one of these
// resolves identically to what the leftmost rule answered before the hop count
// existed. That is what makes the fix a no-op for the documented deployment.
func TestASingleElementChainResolvesTheSameAsItAlwaysDid(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"x-forwarded-for", map[string]string{"X-Forwarded-For": "203.0.113.9"}, "203.0.113.9"},
		{"x-forwarded-for with port", map[string]string{"X-Forwarded-For": "203.0.113.9:51234"}, "203.0.113.9"},
		{"forwarded directive", map[string]string{"Forwarded": `for=203.0.113.7;proto=https`}, "203.0.113.7"},
		{"forwarded quoted ipv6 with port", map[string]string{"Forwarded": `for="[2001:db8::1]:8080"`}, "2001:db8::1"},
		{"x-forwarded-for preferred over forwarded", map[string]string{"X-Forwarded-For": "203.0.113.9", "Forwarded": "for=203.0.113.7"}, "203.0.113.9"},
		{"no forwarding header", nil, "198.51.100.10"},
		{"empty forwarding header", map[string]string{"X-Forwarded-For": "   "}, "198.51.100.10"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveClientIP(trustedRequest(tc.headers), true, 1); got != tc.want {
				t.Fatalf("ResolveClientIP(trusted) = %q, want %q", got, tc.want)
			}
		})
	}
}

// The leftmost element is the client's own claim: every appending proxy keeps
// what the client sent and adds the address it saw after it. Under one trusted
// hop the identity is that appended address, so a forged prefix — of any length,
// including one built to look like a longer chain — changes nothing.
func TestAForgedClientMostElementCannotChooseTheIdentity(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"one forged element", map[string]string{"X-Forwarded-For": "203.0.113.9, 10.0.0.1"}, "10.0.0.1"},
		{"a forged chain", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8, 203.0.113.9, 10.0.0.1"}, "10.0.0.1"},
		{"forged forwarded elements", map[string]string{"Forwarded": `for=203.0.113.7, for=10.0.0.1`}, "10.0.0.1"},
		{"forged element with a port", map[string]string{"X-Forwarded-For": "203.0.113.9, 10.0.0.1:9000"}, "10.0.0.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveClientIP(trustedRequest(tc.headers), true, 1); got != tc.want {
				t.Fatalf("ResolveClientIP = %q, want the address the trusted proxy appended, %q", got, tc.want)
			}
		})
	}
}

// Two trusted hops — a CDN in front of an ingress — is the shape rightmost-only
// gets wrong: the rightmost element is then the CDN's own address, not the
// client's.
func TestTheHopCountWalksBackThroughAChainOfTrustedProxies(t *testing.T) {
	headers := map[string]string{"X-Forwarded-For": "203.0.113.9, 198.51.100.77, 192.0.2.5"}

	if got := ResolveClientIP(trustedRequest(headers), true, 2); got != "198.51.100.77" {
		t.Fatalf("two hops resolved %q, want the address the CDN appended", got)
	}
	if got := ResolveClientIP(trustedRequest(headers), true, 3); got != "203.0.113.9" {
		t.Fatalf("three hops resolved %q, want the client-most element", got)
	}
}

// A chain shorter than the count fails closed to the transport peer rather than
// reaching for whichever element is left. It is the only fail-closed the count
// gives; see the over-configured test below for what it cannot give.
func TestAChainShorterThanTheHopCountFallsBackToThePeer(t *testing.T) {
	cases := []struct {
		name  string
		hops  int
		value string
	}{
		{"one element, two hops configured", 2, "203.0.113.9"},
		{"two elements, three hops configured", 3, "203.0.113.9, 10.0.0.1"},
		{"a forged single element", 2, "203.0.113.9"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := trustedRequest(map[string]string{"X-Forwarded-For": tc.value})
			if got := ResolveClientIP(req, true, tc.hops); got != "198.51.100.10" {
				t.Fatalf("ResolveClientIP = %q, want the peer address: an under-configured hop count must not accept a short chain", got)
			}
		})
	}
}

// Zero is what a RuntimeConfig built without the key carries, and it must mean
// the documented default rather than "no hops", which would read the element the
// client wrote.
func TestAnUnsetHopCountBehavesAsOneTrustedProxy(t *testing.T) {
	headers := map[string]string{"X-Forwarded-For": "203.0.113.9, 10.0.0.1"}

	for _, hops := range []int{0, -1} {
		if got := ResolveClientIP(trustedRequest(headers), true, hops); got != "10.0.0.1" {
			t.Fatalf("hops=%d resolved %q, want the one-trusted-proxy answer", hops, got)
		}
	}
}

func trustedRequest(headers map[string]string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:43123"
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req
}

func TestClientIPReadsTheResolvedValueFromTheContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:43123"
	req = req.WithContext(WithClientIP(req.Context(), "203.0.113.9"))

	if got := ClientIP(req); got != "203.0.113.9" {
		t.Fatalf("ClientIP() = %q, want the resolved value", got)
	}
}

// Absent is not empty: a request that ran through no resolving chain must answer
// the peer address, which is what every audit line and limiter did before the
// trusted-proxy model reached client identity.
func TestClientIPFallsBackToThePeerWhenNothingResolvedOne(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:43123"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")

	if got := ClientIP(req); got != "198.51.100.10" {
		t.Fatalf("ClientIP() = %q, want the peer address", got)
	}

	resolved := req.WithContext(WithClientIP(context.Background(), ""))
	if got := ClientIP(resolved); got != "" {
		t.Fatalf("ClientIP() = %q; a resolved empty value must not be re-read as absent", got)
	}
}

// clientIdentityHeaders are the headers a caller can set to claim an identity.
// Exactly one file may read one: a second reader is a second trust decision, and
// the two drift the moment trustProxyHeaders means different things to each.
var clientIdentityHeaders = []string{
	"X-Forwarded-For",
	"X-Real-Ip",
	"X-Client-Ip",
	"Forwarded",
	"CF-Connecting-IP",
	"True-Client-IP",
	"X-Cluster-Client-IP",
	"Fastly-Client-IP",
}

func clientIdentityHeaderReaders(files []srccensus.SourceFile) []string {
	var readers []string
	for _, file := range files {
		for _, header := range clientIdentityHeaders {
			getHeader := regexp.MustCompile(`(?i)\.Header\.Get\(\s*"` + regexp.QuoteMeta(header) + `"\s*\)`)
			if getHeader.MatchString(file.Text) {
				readers = append(readers, file.Name+` ("`+header+`")`)
			}
		}
	}
	sort.Strings(readers)
	return readers
}

func TestOneFileReadsAForwardingHeaderForClientIdentity(t *testing.T) {
	files := srccensus.Tree(t, "../..", 200)
	readers := clientIdentityHeaderReaders(files)

	want := []string{
		`internal/authn/client_ip.go ("Forwarded")`,
		`internal/authn/client_ip.go ("X-Forwarded-For")`,
		`internal/authn/forwarded.go ("Forwarded")`,
	}
	if strings.Join(readers, "\n  ") != strings.Join(want, "\n  ") {
		t.Fatalf("client-identity header readers are:\n  %s\nwant exactly:\n  %s\na client-identity header is any header in clientIdentityHeaders; add new spellings to that list, never work around it, and resolve identity in ResolveClientIP rather than adding a second reader",
			strings.Join(readers, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestClientIdentityHeaderCensusCatchesEverySpellingAndExcludesSchemeHeaders(t *testing.T) {
	for _, header := range clientIdentityHeaders {
		t.Run(header, func(t *testing.T) {
			files := []srccensus.SourceFile{{
				Name: "internal/handlers/planted_reader.go",
				Text: `package handlers; func planted(r *http.Request) { _ = r.Header.Get("` + strings.ToLower(header) + `") }`,
			}}
			readers := clientIdentityHeaderReaders(files)
			if len(readers) != 1 || !strings.Contains(readers[0], header) {
				t.Fatalf("a planted case-insensitive reader for %q escaped the census: %v", header, readers)
			}
		})
	}

	for _, header := range []string{"X-Forwarded-Proto", "X-Forwarded-Host"} {
		files := []srccensus.SourceFile{{
			Name: "internal/handlers/non_identity_reader.go",
			Text: `package handlers; func planted(r *http.Request) { _ = r.Header.Get("` + header + `") }`,
		}}
		if readers := clientIdentityHeaderReaders(files); len(readers) != 0 {
			t.Fatalf("%q carries connection metadata, not client identity, but the census matched it: %v", header, readers)
		}
	}
}

func TestOnlyTheClientIPResolverReadsTheForwardedForDirective(t *testing.T) {
	pkg := srccensus.Load(t, ".", 5)

	var callers []string
	for _, site := range pkg.Calls(t, "forwardedDirective") {
		callers = append(callers, site.Func)
	}
	sort.Strings(callers)

	want := "RequestHost,RequestScheme,forwardedClientIP"
	if got := strings.Join(callers, ","); got != want {
		t.Fatalf("forwardedDirective is called from %q, want %q; the Forwarded 'for' directive is client identity and forwardedClientIP owns it", got, want)
	}
}

// The hazard the docs must not soften: the server cannot tell a client-written
// element from a proxy-appended one, so a count higher than the proxies that
// really append lets a client pad the chain to that length and choose the
// identity. One appending proxy (peer 198.51.100.10 appends 10.0.0.1) with the
// count set to two or three resolves to whatever the client put in front.
func TestAnOverConfiguredHopCountHandsTheIdentityBackToAPaddingClient(t *testing.T) {
	cases := []struct {
		name  string
		hops  int
		value string
		want  string
	}{
		{"two hops, client pads one element", 2, "203.0.113.9, 10.0.0.1", "203.0.113.9"},
		{"three hops, client pads two elements", 3, "203.0.113.9, 5.6.7.8, 10.0.0.1", "203.0.113.9"},
		{"one hop, the correct count, is not fooled", 1, "203.0.113.9, 5.6.7.8, 10.0.0.1", "10.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveClientIP(trustedRequest(map[string]string{"X-Forwarded-For": tc.value}), true, tc.hops)
			if got != tc.want {
				t.Fatalf("ResolveClientIP = %q, want %q; the docs promise exactly this", got, tc.want)
			}
		})
	}
}
