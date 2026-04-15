package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/netguard"
)

func stubNavigateHostResolution(t *testing.T, fn func(context.Context, string, string) ([]net.IP, error)) {
	t.Helper()
	old := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = fn
	t.Cleanup(func() {
		netguard.ResolveHostIPs = old
	})
}

func TestHandleNavigate_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleNavigate_EnsureChromeFailureStopsBeforeCreateTab(t *testing.T) {
	m := &mockBridge{ensureChromeErr: fmt.Errorf("bridge init failed")}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"about:blank"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if m.ensureChromeCall != 1 {
		t.Fatalf("expected EnsureChrome to be called once, got %d", m.ensureChromeCall)
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called when EnsureChrome fails, got %v", m.createTabURLs)
	}
	if !strings.Contains(w.Body.String(), "chrome initialization") {
		t.Fatalf("expected chrome initialization error, got %s", w.Body.String())
	}
}

func TestHandleTab_EnsureChromeFailureStopsBeforeCreateTab(t *testing.T) {
	m := &mockBridge{ensureChromeErr: fmt.Errorf("bridge init failed")}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(`{"action":"new","url":"about:blank"}`)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if m.ensureChromeCall != 1 {
		t.Fatalf("expected EnsureChrome to be called once, got %d", m.ensureChromeCall)
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called when EnsureChrome fails, got %v", m.createTabURLs)
	}
	if !strings.Contains(w.Body.String(), "chrome initialization") {
		t.Fatalf("expected chrome initialization error, got %s", w.Body.String())
	}
}

func TestValidateNavigateURL_RejectsUnsupportedSchemes(t *testing.T) {
	for _, rawURL := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"chrome://settings",
		"data:text/html,hello",
	} {
		if err := validateNavigateURL(rawURL); err == nil {
			t.Fatalf("validateNavigateURL(%q) should reject unsupported schemes", rawURL)
		}
	}
}

func TestValidateNavigateTarget_AllowsLocalHosts(t *testing.T) {
	for _, rawURL := range []string{
		"http://localhost:9867",
		"http://127.0.0.1:8080",
		"http://[::1]:9222",
		"http://foo.localhost:3000",
		"about:blank",
	} {
		target, err := validateNavigateTarget(rawURL, false, nil)
		if err != nil {
			t.Fatalf("validateNavigateTarget(%q) error = %v", rawURL, err)
		}
		if target == nil || !target.allowInternal {
			t.Fatalf("validateNavigateTarget(%q) should allow local targets", rawURL)
		}
	}
}

func TestValidateNavigateTarget_RejectsPrivateLiteralIP(t *testing.T) {
	if _, err := validateNavigateTarget("http://192.168.1.10/app", false, nil); err == nil {
		t.Fatal("validateNavigateTarget should reject private literal IPs")
	}
}

func TestValidateNavigateTarget_RejectsResolvedPrivateIP(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.10")}, nil
	})

	if _, err := validateNavigateTarget("https://example.com/app", false, nil); err == nil {
		t.Fatal("validateNavigateTarget should reject hosts resolving to private IPs")
	}
}

func TestValidateNavigateTarget_AllowsResolvedPrivateIPWhenExplicitlyAllowlisted(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("172.18.0.5")}, nil
	})

	target, err := validateNavigateTarget("http://fixtures:80/app", true, nil)
	if err != nil {
		t.Fatalf("validateNavigateTarget should allow explicitly allowlisted private targets: %v", err)
	}
	if target == nil || !target.allowInternal {
		t.Fatal("validateNavigateTarget should mark explicitly allowlisted private targets as allowed")
	}
}

func TestValidateNavigateURL_AllowsHTTPHTTPSAndBareHostnames(t *testing.T) {
	for _, rawURL := range []string{
		"https://pinchtab.com",
		"http://pinchtab.test",
		"pinchtab.com",
		"about:blank",
	} {
		if err := validateNavigateURL(rawURL); err != nil {
			t.Fatalf("validateNavigateURL(%q) error = %v", rawURL, err)
		}
	}
}

func TestValidateNavigateURL_RejectsOverlongURL(t *testing.T) {
	rawURL := "https://pinchtab.com/" + strings.Repeat("a", maxNavigateURLLen)
	if err := validateNavigateURL(rawURL); err == nil {
		t.Fatal("validateNavigateURL should reject overlong urls")
	}
}

func TestHandleNavigate_RejectsUnsupportedSchemeBeforeCreateTab(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"file:///etc/passwd"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called for rejected schemes, got %v", m.createTabURLs)
	}
	if !strings.Contains(w.Body.String(), "invalid URL scheme") {
		t.Fatalf("expected invalid URL scheme error, got %s", w.Body.String())
	}
}

func TestHandleNavigate_RejectsUnsupportedSchemeForExistingTab(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"tabId":"tab1","url":"javascript:alert(1)"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid URL scheme") {
		t.Fatalf("expected invalid URL scheme error, got %s", w.Body.String())
	}
}

func TestValidateNavigateTarget_AllowsPrivateIPWithTrustedResolveCIDR(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	trusted := parseCIDRs([]string{"10.0.0.0/8"})
	target, err := validateNavigateTarget("https://internal.example.com", false, trusted)
	if err != nil {
		t.Fatalf("expected trusted CIDR to allow private IP, got %v", err)
	}
	if target == nil || target.allowInternal {
		t.Fatal("trusted CIDR override should not set allowInternal (runtime guard should still be active)")
	}
	if len(target.trustedResolvedIP) != 1 || target.trustedResolvedIP[0] != netip.MustParseAddr("10.0.0.5") {
		t.Fatalf("expected exact trusted resolved IPs to be captured, got %v", target.trustedResolvedIP)
	}
}

func TestValidateNavigateTarget_RejectsMixedUntrustedWithTrustedResolveCIDR(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5"), net.ParseIP("192.168.1.1")}, nil
	})

	trusted := parseCIDRs([]string{"10.0.0.0/8"})
	if _, err := validateNavigateTarget("https://mixed.example.com", false, trusted); err == nil {
		t.Fatal("expected mixed trusted/untrusted private IPs to be blocked")
	}
}

func TestHandleNavigate_AllowsPrivateIPWithTrustedResolveCIDR(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("198.18.0.10")}, nil
	})

	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{
		TrustedResolveCIDRs: []string{"198.18.0.0/15"},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"https://benchmark.example.com"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == 403 {
		t.Fatalf("expected trusted resolve CIDR to allow navigation, got 403: %s", w.Body.String())
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected CreateTab to be called for trusted resolve CIDR navigation")
	}
}

func TestValidateNavigateRemoteIPAddress_AllowsExactTrustedResolvedIP(t *testing.T) {
	if err := validateNavigateRemoteIPAddress("10.1.2.3", nil, []netip.Addr{netip.MustParseAddr("10.1.2.3")}); err != nil {
		t.Fatalf("expected exact trusted resolved IP to be allowed, got %v", err)
	}
}

func TestValidateNavigateRemoteIPAddress_RejectsDifferentIPInSameCIDR(t *testing.T) {
	err := validateNavigateRemoteIPAddress("10.1.2.4", nil, []netip.Addr{netip.MustParseAddr("10.1.2.3")})
	if err == nil {
		t.Fatal("expected different runtime IP in same CIDR to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked remote IP") {
		t.Fatalf("expected blocked remote IP error, got %v", err)
	}
}

func TestParseCIDRs_TreatsBareIPsAsSingleHosts(t *testing.T) {
	cidrs := parseCIDRs([]string{"10.1.2.3", "fd00::1234"})
	if len(cidrs) != 2 {
		t.Fatalf("parseCIDRs() returned %d entries, want 2", len(cidrs))
	}
	if got := cidrs[0].String(); got != "10.1.2.3/32" {
		t.Fatalf("IPv4 bare IP parsed as %q, want 10.1.2.3/32", got)
	}
	if got := cidrs[1].String(); got != "fd00::1234/128" {
		t.Fatalf("IPv6 bare IP parsed as %q, want fd00::1234/128", got)
	}
}

func TestHandleNavigate_AllowsLocalhostWithoutResolver(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"http://localhost:3000"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected localhost navigate to proceed, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected CreateTab to be called for localhost navigate")
	}
}

func TestHandleNavigate_RejectsResolvedPrivateIPBeforeCreateTab(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"https://example.com"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called for blocked targets, got %v", m.createTabURLs)
	}
	if !strings.Contains(w.Body.String(), "blocked private/internal IP") {
		t.Fatalf("expected blocked private/internal IP error, got %s", w.Body.String())
	}
}

func TestHandleNavigate_AllowsResolvedPrivateIPWhenIDPIAllowlisted(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("172.18.0.5")}, nil
	})

	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{
		AllowedDomains: []string{"fixtures"},
		IDPI: config.IDPIConfig{
			Enabled:    true,
			StrictMode: true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"http://fixtures:80/buttons.html"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected allowlisted internal navigate to proceed, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected CreateTab to be called for allowlisted internal navigate")
	}
}

func TestHandleNavigate_RejectsOverlongURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	longURL := "https://pinchtab.com/" + strings.Repeat("a", maxNavigateURLLen)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"`+longURL+`"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "url too long") {
		t.Fatalf("expected url too long error, got %s", w.Body.String())
	}
}

func TestHandleTabNavigate_MissingTabID(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//navigate", bytes.NewReader([]byte(`{"url":"https://pinchtab.com"}`)))
	w := httptest.NewRecorder()
	h.HandleTabNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabNavigate_TabIDMismatch(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/navigate", bytes.NewReader([]byte(`{"tabId":"tab_other","url":"https://pinchtab.com"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_InvalidAction(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"action":"invalid"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_CloseMissingID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"action":"close"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_RejectsUnsupportedScheme(t *testing.T) {
	for _, scheme := range []string{"file:///etc/passwd", "javascript:alert(1)", "chrome://settings"} {
		m := &mockBridge{}
		h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
		body := `{"action":"new","url":"` + scheme + `"}`
		req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		h.HandleTab(w, req)
		if w.Code != 400 {
			t.Errorf("scheme %q: expected 400, got %d: %s", scheme, w.Code, w.Body.String())
		}
		if len(m.createTabURLs) != 0 {
			t.Errorf("scheme %q: CreateTab should not be called but was", scheme)
		}
	}
}

func TestHandleTab_RejectsPrivateLiteralIP(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"action":"new","url":"http://192.168.1.1/"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called for blocked targets")
	}
}

func TestHandleTab_RejectsResolvedPrivateIP(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"action":"new","url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called for blocked targets")
	}
}

func TestHandleTab_AllowsValidURL(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"action":"new","url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected valid URL to proceed, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected CreateTab to be called for valid URL")
	}
}

func TestIsNavigateAbortedOnBinary(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		url    string
		expect bool
	}{
		{"gz file with ERR_ABORTED", fmt.Errorf("net::ERR_ABORTED"), "https://example.com/file.gz", true},
		{"xml.gz file with ERR_ABORTED", fmt.Errorf("net::ERR_ABORTED"), "https://example.com/sitemap.xml.gz", true},
		{"zip file with ERR_ABORTED", fmt.Errorf("net::ERR_ABORTED"), "https://example.com/archive.zip", true},
		{"pdf file with ERR_ABORTED", fmt.Errorf("net::ERR_ABORTED"), "https://example.com/doc.pdf", true},
		{"gz with query param", fmt.Errorf("net::ERR_ABORTED"), "https://example.com/file.gz?token=abc", true},
		{"html file with ERR_ABORTED", fmt.Errorf("net::ERR_ABORTED"), "https://example.com/page.html", false},
		{"gz file without ERR_ABORTED", fmt.Errorf("net::ERR_CONNECTION_REFUSED"), "https://example.com/file.gz", false},
		{"html file different error", fmt.Errorf("timeout"), "https://example.com/page.html", false},
		{"nil error", nil, "https://example.com/file.gz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNavigateAbortedOnBinary(tt.err, tt.url)
			if result != tt.expect {
				t.Errorf("isNavigateAbortedOnBinary(%v, %q) = %v, want %v", tt.err, tt.url, result, tt.expect)
			}
		})
	}
}
