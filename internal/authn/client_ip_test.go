package authn

import (
	"context"
	"net/http/httptest"
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

	if got := ResolveClientIP(req, false); got != "198.51.100.10" {
		t.Fatalf("ResolveClientIP(untrusted) = %q, want the peer address", got)
	}
}

func TestResolveClientIPTakesTheClientMostForwardedEntryWhenProxyIsTrusted(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"x-forwarded-for chain", map[string]string{"X-Forwarded-For": "203.0.113.9, 10.0.0.1"}, "203.0.113.9"},
		{"x-forwarded-for with port", map[string]string{"X-Forwarded-For": "203.0.113.9:51234"}, "203.0.113.9"},
		{"forwarded directive", map[string]string{"Forwarded": `for=203.0.113.7;proto=https`}, "203.0.113.7"},
		{"forwarded client-most element", map[string]string{"Forwarded": `for=203.0.113.7, for=10.0.0.1`}, "203.0.113.7"},
		{"forwarded quoted ipv6 with port", map[string]string{"Forwarded": `for="[2001:db8::1]:8080"`}, "2001:db8::1"},
		{"x-forwarded-for preferred over forwarded", map[string]string{"X-Forwarded-For": "203.0.113.9", "Forwarded": "for=203.0.113.7"}, "203.0.113.9"},
		{"no forwarding header", nil, "198.51.100.10"},
		{"empty forwarding header", map[string]string{"X-Forwarded-For": "   "}, "198.51.100.10"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "198.51.100.10:43123"
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}

			if got := ResolveClientIP(req, true); got != tc.want {
				t.Fatalf("ResolveClientIP(trusted) = %q, want %q", got, tc.want)
			}
		})
	}
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
var clientIdentityHeaders = []string{`"X-Forwarded-For"`, `"X-Real-Ip"`, `"X-Real-IP"`, `"X-Client-Ip"`}

func TestOneFileReadsAForwardingHeaderForClientIdentity(t *testing.T) {
	files := srccensus.Tree(t, "../..", 200)

	var readers []string
	for _, file := range files {
		for _, header := range clientIdentityHeaders {
			if strings.Contains(file.Text, header) {
				readers = append(readers, file.Name+" ("+header+")")
				break
			}
		}
	}

	want := []string{`internal/authn/client_ip.go ("X-Forwarded-For")`}
	sort.Strings(readers)
	if strings.Join(readers, "\n  ") != strings.Join(want, "\n  ") {
		t.Fatalf("client-identity header readers are:\n  %s\nwant exactly:\n  %s\nresolve the identity in ResolveClientIP and read it back with ClientIP rather than adding a second reader",
			strings.Join(readers, "\n  "), strings.Join(want, "\n  "))
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
