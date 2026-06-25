package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestNormalizeCDPURL_BrowserWebSocket(t *testing.T) {
	in := "ws://127.0.0.1:9222/devtools/browser/abc123"
	got, err := NormalizeCDPURL(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != in {
		t.Fatalf("got %q, want %q", got, in)
	}
}

func TestNormalizeCDPURL_RejectsPageLevel(t *testing.T) {
	in := "ws://127.0.0.1:9222/devtools/page/xyz"
	_, err := NormalizeCDPURL(in)
	if err == nil {
		t.Fatalf("expected error for page-level URL")
	}
	if !strings.Contains(err.Error(), "page-level") {
		t.Fatalf("error should mention page-level, got %v", err)
	}
}

func TestNormalizeCDPURL_RejectsEmpty(t *testing.T) {
	if _, err := NormalizeCDPURL(""); err == nil {
		t.Fatalf("expected error for empty cdpUrl")
	}
	if _, err := NormalizeCDPURL("   "); err == nil {
		t.Fatalf("expected error for whitespace cdpUrl")
	}
}

func TestNormalizeCDPURL_RejectsUnsupportedScheme(t *testing.T) {
	if _, err := NormalizeCDPURL("ftp://x/devtools/browser/y"); err == nil {
		t.Fatalf("expected error for ftp scheme")
	}
}

func TestNormalizeCDPURL_RejectsNonLoopbackByDefault(t *testing.T) {
	_, err := NormalizeCDPURL("ws://169.254.169.254:9222/devtools/browser/abc")
	if err == nil {
		t.Fatalf("expected non-loopback remote CDP host to be rejected")
	}
	if !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("error should mention non-loopback, got %v", err)
	}
}

func TestNormalizeCDPURL_AllowsExplicitlyAllowlistedRemoteHost(t *testing.T) {
	got, err := NormalizeCDPURLWithAllowlist("ws://192.0.2.10:9222/devtools/browser/abc", []string{"192.0.2.10"}, []string{"ws"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws://192.0.2.10:9222/devtools/browser/abc" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeCDPURL_RejectsDisallowedAttachScheme(t *testing.T) {
	_, err := NormalizeCDPURLWithAllowlist("http://127.0.0.1:9222", []string{"127.0.0.1"}, []string{"ws"})
	if err == nil {
		t.Fatal("expected disallowed scheme to be rejected")
	}
	if !strings.Contains(err.Error(), "scheme") || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error should mention disallowed scheme, got %v", err)
	}
}

func TestNormalizeCDPURL_RejectsWsWithoutBrowserPath(t *testing.T) {
	if _, err := NormalizeCDPURL("ws://127.0.0.1:9222/something/else"); err == nil {
		t.Fatalf("expected error for missing /devtools/browser/")
	}
}

func TestNormalizeCDPURL_HTTPOriginResolves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/test-id"}`, r.Host)
	}))
	defer srv.Close()

	got, err := NormalizeCDPURL(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "ws://") || !strings.Contains(got, "/devtools/browser/") {
		t.Fatalf("expected resolved browser WS URL, got %q", got)
	}
}

func TestNormalizeCDPURL_HTTPOriginRewritesLoopbackDebuggerHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"webSocketDebuggerUrl":"ws://[::1]:9222/devtools/browser/test-id"}`)
	}))
	defer srv.Close()

	got, err := NormalizeCDPURL(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/devtools/browser/test-id"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeCDPURL_HTTPJSONVersionResolves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/test-id"}`, r.Host)
	}))
	defer srv.Close()

	got, err := NormalizeCDPURL(srv.URL + "/json/version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "ws://") {
		t.Fatalf("expected ws:// URL, got %q", got)
	}
}

func TestProbeCDPVersionHTTPOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/test-id"}`, r.Host)
	}))
	defer srv.Close()

	got, err := ProbeCDPVersion(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.VersionURL != srv.URL+"/json/version" {
		t.Fatalf("VersionURL = %q, want %q", got.VersionURL, srv.URL+"/json/version")
	}
	if !strings.Contains(got.WebSocketDebuggerURL, "/devtools/browser/test-id") {
		t.Fatalf("WebSocketDebuggerURL = %q", got.WebSocketDebuggerURL)
	}
}

func TestProbeCDPVersionBrowserWebSocket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/test-id"}`, r.Host)
	}))
	defer srv.Close()

	raw := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/devtools/browser/test-id"
	got, err := ProbeCDPVersion(context.Background(), raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.VersionURL != srv.URL+"/json/version" {
		t.Fatalf("VersionURL = %q, want %q", got.VersionURL, srv.URL+"/json/version")
	}
	if got.WebSocketDebuggerURL != raw {
		t.Fatalf("WebSocketDebuggerURL = %q, want %q", got.WebSocketDebuggerURL, raw)
	}
}

func TestNormalizeCDPURL_HTTPRejectsArbitraryPath(t *testing.T) {
	if _, err := NormalizeCDPURL("http://127.0.0.1:9222/some/other/path"); err == nil {
		t.Fatalf("expected error for arbitrary HTTP path")
	}
}

func TestNormalizeCDPURL_HTTPDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NormalizeCDPURL(srv.URL)
	if err == nil {
		t.Fatalf("expected redirect response to fail")
	}
	if !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("error should report redirect status, got %v", err)
	}
}

func TestNormalizeCDPURL_HTTPVersionBodyIsCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", cdpVersionBodyLimit+1)))
	}))
	defer srv.Close()

	_, err := NormalizeCDPURL(srv.URL)
	if err == nil {
		t.Fatalf("expected oversized response to fail")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error should mention body limit, got %v", err)
	}
}

func TestNormalizeResolvedDevToolsURL_AllowsRequestedPinnedIP(t *testing.T) {
	requested, err := url.Parse("http://cdp.example.test:9222")
	if err != nil {
		t.Fatalf("parse requested URL: %v", err)
	}

	got, err := normalizeResolvedDevToolsURL(
		"ws://192.0.2.10:9222/devtools/browser/abc",
		requested,
		netip.MustParseAddr("192.0.2.10"),
		[]string{"cdp.example.test"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws://192.0.2.10:9222/devtools/browser/abc" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeResolvedDevToolsURL_RejectsAllowlistedDifferentHost(t *testing.T) {
	requested, err := url.Parse("http://192.0.2.10:9222")
	if err != nil {
		t.Fatalf("parse requested URL: %v", err)
	}

	_, err = normalizeResolvedDevToolsURL(
		"ws://192.0.2.20:9222/devtools/browser/abc",
		requested,
		netip.MustParseAddr("192.0.2.10"),
		[]string{"192.0.2.20"},
	)
	if err == nil {
		t.Fatal("expected different allowlisted response host to be rejected")
	}
	if !strings.Contains(err.Error(), "does not match requested CDP endpoint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubCDPResolver(t *testing.T, fn func(ctx context.Context, network, host string) ([]net.IP, error)) {
	t.Helper()
	old := cdpResolveHostIPs
	cdpResolveHostIPs = fn
	t.Cleanup(func() { cdpResolveHostIPs = old })
}

// H9 regression: an allowlisted DNS hostname must pass the connectivity
// probe's allowlist check. Normalization pins the URL host to the resolved
// IP, and probing that rewritten URL re-validated the bare IP against the
// hostname allowlist — which can never match. The dial here still fails (no
// live endpoint), but the failure must be a connectivity error, not the
// pre-fix "add it to security.attach.allowHosts" rejection.
func TestInitRemoteCDP_AllowlistedHostnamePassesProbeAllowlist(t *testing.T) {
	stubCDPResolver(t, func(_ context.Context, _, host string) ([]net.IP, error) {
		if host == "browsers.corp" {
			return []net.IP{net.ParseIP("203.0.113.10")}, nil
		}
		return nil, fmt.Errorf("unexpected host %q", host)
	})

	cfg := &config.RuntimeConfig{
		AttachAllowHosts:   []string{"browsers.corp"},
		AttachAllowSchemes: []string{"ws"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, _, _, _, _, err := InitRemoteCDP(ctx, cfg, "ws://browsers.corp:9222/devtools/browser/abc")
	if err == nil {
		t.Fatal("expected connectivity failure against unreachable endpoint")
	}
	if strings.Contains(err.Error(), "allowHosts") {
		t.Fatalf("allowlisted hostname rejected by the probe's allowlist re-check: %v", err)
	}
}

func TestInitRemoteCDP_NonAllowlistedHostnameStillRejected(t *testing.T) {
	stubCDPResolver(t, func(_ context.Context, _, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.11")}, nil
	})

	cfg := &config.RuntimeConfig{
		AttachAllowHosts:   []string{"browsers.corp"},
		AttachAllowSchemes: []string{"ws"},
	}
	_, _, _, _, _, err := InitRemoteCDP(context.Background(), cfg, "ws://evil.corp:9222/devtools/browser/abc")
	if err == nil {
		t.Fatal("expected non-allowlisted hostname to be rejected")
	}
	if !strings.Contains(err.Error(), "allowHosts") {
		t.Fatalf("expected allowlist rejection from normalization, got: %v", err)
	}
}
