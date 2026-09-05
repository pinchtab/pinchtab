package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/navguard"
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

func TestHandleNavigate_EnsureBrowserFailureStopsBeforeCreateTab(t *testing.T) {
	m := &mockBridge{ensureBrowserErr: fmt.Errorf("bridge init failed")}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"about:blank"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if m.ensureBrowserCall != 1 {
		t.Fatalf("expected EnsureBrowser to be called once, got %d", m.ensureBrowserCall)
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called when EnsureBrowser fails, got %v", m.createTabURLs)
	}
	if !strings.Contains(w.Body.String(), "browser initialization") {
		t.Fatalf("expected browser initialization error, got %s", w.Body.String())
	}
}

func TestHandleTab_EnsureBrowserFailureStopsBeforeCreateTab(t *testing.T) {
	m := &mockBridge{ensureBrowserErr: fmt.Errorf("bridge init failed")}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(`{"action":"new","url":"about:blank"}`)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if m.ensureBrowserCall != 1 {
		t.Fatalf("expected EnsureBrowser to be called once, got %d", m.ensureBrowserCall)
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called when EnsureBrowser fails, got %v", m.createTabURLs)
	}
	if !strings.Contains(w.Body.String(), "browser initialization") {
		t.Fatalf("expected browser initialization error, got %s", w.Body.String())
	}
}

func TestValidateNavigateURL_RejectsUnsupportedSchemes(t *testing.T) {
	for _, rawURL := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"chrome://settings",
		"data:text/html,hello",
	} {
		if err := validateNavigateURL(rawURL, false); err == nil {
			t.Fatalf("validateNavigateURL(%q) should reject unsupported schemes", rawURL)
		}
	}
}

func TestValidateNavigateURL_GatesFileScheme(t *testing.T) {
	const fileURL = "file:///tmp/pinchtab.html"
	if err := validateNavigateURL(fileURL, false); err == nil {
		t.Fatal("validateNavigateURL(file, false) should reject file:// when not opted in")
	}
	if err := validateNavigateURL(fileURL, true); err != nil {
		t.Fatalf("validateNavigateURL(file, true) error = %v", err)
	}
	// The opt-in is scoped to file:// only; other schemes stay rejected.
	for _, rawURL := range []string{"javascript:alert(1)", "chrome://settings", "data:text/html,hello"} {
		if err := validateNavigateURL(rawURL, true); err == nil {
			t.Fatalf("validateNavigateURL(%q, true) should still reject", rawURL)
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
		if target == nil || !target.AllowInternal {
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

func TestValidateNavigateTarget_AllowsDualStackPublicResolution(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{
			net.ParseIP("2606:2800:220:1:248:1893:25c8:1946"),
			net.ParseIP("93.184.216.34"),
		}, nil
	})

	if _, err := validateNavigateTarget("https://dual.example/app", false, nil); err != nil {
		t.Fatalf("validateNavigateTarget should allow public dual-stack resolution: %v", err)
	}
}

func TestValidateNavigateTarget_RejectsMixedPublicPrivateDualStackResolution(t *testing.T) {
	tests := []struct {
		name string
		ips  []net.IP
	}{
		{
			name: "private-v6-first",
			ips:  []net.IP{net.ParseIP("fd00::10"), net.ParseIP("93.184.216.34")},
		},
		{
			name: "public-v4-first",
			ips:  []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("fd00::10")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
				return tt.ips, nil
			})

			if _, err := validateNavigateTarget("https://mixed.example/app", false, nil); err == nil {
				t.Fatal("validateNavigateTarget should reject mixed public/private DNS answers")
			}
		})
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
	if target == nil {
		t.Fatal("validateNavigateTarget returned nil target")
	}
	if target.AllowInternal {
		t.Fatal("allowlisted private targets should not disable redirect/internal-IP runtime guards")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("172.18.0.5") {
		t.Fatalf("trustedResolvedIP = %v, want [172.18.0.5]", target.TrustedResolvedIP)
	}
}

func TestValidateNavigateURL_AllowsHTTPHTTPSAndBareHostnames(t *testing.T) {
	for _, rawURL := range []string{
		"https://pinchtab.com",
		"http://pinchtab.test",
		"pinchtab.com",
		"about:blank",
	} {
		if err := validateNavigateURL(rawURL, false); err != nil {
			t.Fatalf("validateNavigateURL(%q) error = %v", rawURL, err)
		}
	}
}

func TestValidateNavigateURL_RejectsOverlongURL(t *testing.T) {
	rawURL := "https://pinchtab.com/" + strings.Repeat("a", navguard.MaxURLLen)
	if err := validateNavigateURL(rawURL, false); err == nil {
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

func TestHandleNavigate_AllowsFileSchemeWhenEnabled(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{AllowFileScheme: true}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"file:///tmp/pinchtab.html"}`)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected file scheme navigate to proceed past validation, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected CreateTab to be called for opted-in file scheme navigation")
	}
}

func TestHandleTab_AllowsFileSchemeWhenEnabled(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{AllowFileScheme: true}, nil, nil, nil)

	body := `{"action":"new","url":"file:///tmp/pinchtab.html"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected file scheme tab creation to proceed, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected CreateTab to be called for opted-in file scheme tab creation")
	}
}

func TestHandleTab_RejectsFileSchemeWhenDisabled(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"action":"new","url":"file:///etc/passwd"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for file scheme when AllowFileScheme is off, got %d: %s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("CreateTab should not be called for rejected file scheme, got %v", m.createTabURLs)
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

func TestHandleNavigateDispatchOnlyReturnsAfterAcceptedBackgroundNavigation(t *testing.T) {
	m := &mockBridge{navigateResult: &bridge.NavigateResult{
		URL: "http://localhost:8765/slow-spa",
	}}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(
		`{"tabId":"tab1","url":"http://localhost:8765/slow-spa","dispatchOnly":true}`,
	)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != 200 {
		t.Fatalf("dispatch-only status = %d: %s", w.Code, w.Body.String())
	}
	if len(m.navigateParams) != 1 || !m.navigateParams[0].DispatchOnly {
		t.Fatalf("navigate params = %+v, want DispatchOnly", m.navigateParams)
	}
	if !strings.Contains(w.Body.String(), `"dispatched":true`) {
		t.Fatalf("dispatch-only receipt missing: %s", w.Body.String())
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
	if target == nil || target.AllowInternal {
		t.Fatal("trusted CIDR override should not set allowInternal (runtime guard should still be active)")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("10.0.0.5") {
		t.Fatalf("expected exact trusted resolved IPs to be captured, got %v", target.TrustedResolvedIP)
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
	if err := navguard.ValidateRemoteIP("10.1.2.3", nil, []netip.Addr{netip.MustParseAddr("10.1.2.3")}); err != nil {
		t.Fatalf("expected exact trusted resolved IP to be allowed, got %v", err)
	}
}

func TestValidateNavigateRemoteIPAddress_RejectsDifferentIPInSameCIDR(t *testing.T) {
	err := navguard.ValidateRemoteIP("10.1.2.4", nil, []netip.Addr{netip.MustParseAddr("10.1.2.3")})
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
	longURL := "https://pinchtab.com/" + strings.Repeat("a", navguard.MaxURLLen)

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

func TestHandleTab_CloseActionUnsupported(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"action":"close"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/close") {
		t.Fatalf("expected /close hint, got %s", w.Body.String())
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

func TestNavigateRefusesAnErrorPageLanding(t *testing.T) {
	const target = "http://localhost/page.html"
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	cases := []struct {
		name      string
		netError  string
		retryable bool
	}{
		{"offline tab", "net::ERR_INTERNET_DISCONNECTED", false},
		{"abort route rule", "net::ERR_BLOCKED_BY_CLIENT", false},
		{"dns failure", "net::ERR_NAME_NOT_RESOLVED", false},
		{"connection refused", "net::ERR_CONNECTION_REFUSED", true},
		{"no recorded reason", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockBridge{currentURL: "chrome-error://chromewebdata/", navigateResult: &bridge.NavigateResult{URL: target}, networkMonitor: bridge.NewNetworkMonitor(10)}
			if tc.netError != "" {
				m.networkMonitor.GetOrCreateBufferForTest("tab1").Add(bridge.NetworkEntry{URL: target, Failed: true, Error: tc.netError})
			}
			h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
			srv := httptest.NewServer(http.HandlerFunc(h.HandleNavigate))
			defer srv.Close()

			body := map[string]any{"url": target, "tabId": "tab1"}
			status, raw, _ := apiclient.DoPostQuietWithStatus(srv.Client(), srv.URL, "", "/navigate", body)
			if status != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502: %s", status, raw)
			}
			var result map[string]any
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatalf("decode %s: %v", raw, err)
			}
			if result["code"] != "navigation_not_loaded" {
				t.Fatalf("code = %v, want navigation_not_loaded: %s", result["code"], raw)
			}
			if retryable, _ := result["retryable"].(bool); retryable != tc.retryable {
				t.Fatalf("retryable = %v, want %v for %q", retryable, tc.retryable, tc.netError)
			}
			msg, _ := result["error"].(string)
			if tc.netError != "" && !strings.Contains(msg, tc.netError) {
				t.Fatalf("error %q does not name the recorded reason %q", msg, tc.netError)
			}
			if tc.netError == "" && !strings.Contains(msg, "recorded no reason") {
				t.Fatalf("error %q does not state that no reason was recorded", msg)
			}
			if _, err := apiclient.DoPostRawE(srv.Client(), srv.URL, "", "/navigate", body); err == nil {
				t.Fatal("the CLI's request path treats this navigate as a success, so pinchtab nav would exit 0")
			}
		})
	}

	t.Run("a new tab that lands on the error page is closed, not stranded", func(t *testing.T) {
		m := &mockBridge{currentURL: "chrome-error://chromewebdata/", navigateResult: &bridge.NavigateResult{URL: target}}
		h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
		w := httptest.NewRecorder()
		h.HandleNavigate(w, httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"`+target+`","newTab":true}`))))
		if w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d: %s", w.Code, w.Body.String())
		}
		if len(m.createTabURLs) != 1 || len(m.closedTabs) != 1 {
			t.Fatalf("created %v, closed %v; the tab the failed navigate created must be closed", m.createTabURLs, m.closedTabs)
		}
	})

	t.Run("a loaded page keeps today's 200", func(t *testing.T) {
		m := &mockBridge{currentURL: target, navigateResult: &bridge.NavigateResult{URL: target}}
		h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
		srv := httptest.NewServer(http.HandlerFunc(h.HandleNavigate))
		defer srv.Close()
		body := map[string]any{"url": target, "tabId": "tab1"}
		status, raw, result := apiclient.DoPostQuietWithStatus(srv.Client(), srv.URL, "", "/navigate", body)
		if status != 200 || result["url"] != target || result["tabId"] != "tab1" {
			t.Fatalf("status %d body %s", status, raw)
		}
		if _, ok := result["title"]; !ok {
			t.Fatalf("success body lost its title key: %s", raw)
		}
		if _, err := apiclient.DoPostRawE(srv.Client(), srv.URL, "", "/navigate", body); err != nil {
			t.Fatalf("a loaded page reads as a failure on the CLI path: %v", err)
		}
	})
}

// The tab a newTab navigate creates has one owner deciding its fate: it is closed
// on every exit unless the response names it as the caller's. The rule is the
// response tab, not failure — the ghost-chrome path answers 200 from another tab
// and closes the created one — so all three shapes sit in one table and a
// close-on-failure or close-everything regression is visible.
func TestANewTabIsClosedUnlessTheResponseNamesIt(t *testing.T) {
	const target = "http://localhost/page.html"
	cases := []struct {
		name       string
		mock       *mockBridge
		wantStatus int
		wantTabID  string
		wantClosed bool
	}{
		{
			name:       "a chrome-error landing closes the created tab and names none",
			mock:       &mockBridge{currentURL: "chrome-error://chromewebdata/", navigateResult: &bridge.NavigateResult{URL: target}},
			wantStatus: http.StatusBadGateway, wantTabID: "", wantClosed: true,
		},
		{
			name:       "a navigate error closes the created tab and names none",
			mock:       &mockBridge{navigateErr: context.DeadlineExceeded},
			wantStatus: http.StatusServiceUnavailable, wantTabID: "", wantClosed: true,
		},
		{
			name:       "a loaded page keeps the created tab and names it",
			mock:       &mockBridge{currentURL: target, navigateResult: &bridge.NavigateResult{URL: target}},
			wantStatus: 200, wantTabID: "tab_abc12345", wantClosed: false,
		},
		{
			name:       "a navigate served from another tab answers 200 with that tab and closes the created one",
			mock:       &mockBridge{currentURL: target, navigateResult: &bridge.NavigateResult{TabID: "static-1", URL: target, Title: "served"}},
			wantStatus: 200, wantTabID: "static-1", wantClosed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(tc.mock, &config.RuntimeConfig{}, nil, nil, nil)
			w := httptest.NewRecorder()
			h.HandleNavigate(w, httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"`+target+`","newTab":true}`))))
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if len(tc.mock.createTabURLs) != 1 {
				t.Fatalf("created %v, want exactly one tab", tc.mock.createTabURLs)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			gotTabID, _ := body["tabId"].(string)
			if gotTabID != tc.wantTabID {
				t.Fatalf("response names tab %q, want %q: %s", gotTabID, tc.wantTabID, w.Body.String())
			}
			closed := len(tc.mock.closedTabs) == 1 && tc.mock.closedTabs[0] == "tab_abc12345"
			if closed != tc.wantClosed {
				t.Fatalf("closed = %v (%v), want %v; a created tab must be closed exactly when the response does not hand it over", closed, tc.mock.closedTabs, tc.wantClosed)
			}
		})
	}
}
