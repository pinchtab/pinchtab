package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/netguard"
)

// providerSetup describes how to configure Handlers for a given browser
// provider path.
type providerSetup struct {
	Name  string
	Setup func(t *testing.T, cfg *config.RuntimeConfig) *Handlers
}

// navigateProviders returns the provider paths for /navigate security tests
// (chrome and cloak — ghost-chrome has no parity coverage here yet, a known
// gap). Both share the same navguard+IDPI code paths before any Browser or
// Chrome call is made, so the security checks are exercised uniformly.
func navigateProviders() []providerSetup {
	return []providerSetup{
		{
			Name: "chrome",
			Setup: func(t *testing.T, cfg *config.RuntimeConfig) *Handlers {
				t.Helper()
				cfg.DefaultBrowser = config.BrowserChrome
				return New(&mockBridge{}, cfg, nil, nil, nil)
			},
		},
		{
			Name: "cloak",
			Setup: func(t *testing.T, cfg *config.RuntimeConfig) *Handlers {
				t.Helper()
				cfg.DefaultBrowser = config.BrowserCloak
				cfg.BrowsersAvailable = []string{config.BrowserCloak}
				return New(&mockBridge{}, cfg, nil, nil, nil)
			},
		},
	}
}

// =========================================================================
// Navigation security parity: /navigate endpoint
// =========================================================================

func TestSecurityParity_Navigate_DisallowedDomainBlocked(t *testing.T) {
	// Domain check fires before DNS resolution and before any Browser call,
	// so it works identically across all four providers.
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	for _, p := range navigateProviders() {
		t.Run(p.Name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{
				AllowedDomains: []string{"example.com"},
				IDPI: config.IDPIConfig{
					Enabled:    true,
					StrictMode: true,
				},
			}
			h := p.Setup(t, cfg)

			req := httptest.NewRequest("POST", "/navigate",
				strings.NewReader(`{"url":"https://evil.example.net"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleNavigate(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for disallowed domain, got %d: %s",
					w.Code, w.Body.String())
			}
		})
	}
}

func TestSecurityParity_Navigate_AllowedDomainPermitted(t *testing.T) {
	// Use a local test server and include its address (127.0.0.1) in the
	// allowed domains so the IDPI domain check passes.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>allowed page</body></html>`))
	}))
	defer ts.Close()

	for _, p := range navigateProviders() {
		t.Run(p.Name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{
				AllowedDomains: []string{"example.com", "127.0.0.1"},
				IDPI: config.IDPIConfig{
					Enabled:    true,
					StrictMode: true,
				},
			}
			h := p.Setup(t, cfg)

			req := httptest.NewRequest("POST", "/navigate",
				strings.NewReader(`{"url":"`+ts.URL+`"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleNavigate(w, req)

			// Should NOT be blocked by the domain guard. Downstream failures
			// (200 from Browser, 500 from Chrome mock) are acceptable.
			if w.Code == http.StatusForbidden {
				t.Fatalf("allowed domain should not be blocked, got 403: %s",
					w.Body.String())
			}
		})
	}
}

func TestSecurityParity_Navigate_PrivateIPBlocked(t *testing.T) {
	for _, p := range navigateProviders() {
		t.Run(p.Name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{}
			h := p.Setup(t, cfg)

			req := httptest.NewRequest("POST", "/navigate",
				strings.NewReader(`{"url":"http://192.168.1.1/"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleNavigate(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for private IP, got %d: %s",
					w.Code, w.Body.String())
			}
		})
	}
}

func TestSecurityParity_Navigate_ResolvedPrivateIPBlocked(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	for _, p := range navigateProviders() {
		t.Run(p.Name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{}
			h := p.Setup(t, cfg)

			req := httptest.NewRequest("POST", "/navigate",
				strings.NewReader(`{"url":"https://example.com"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleNavigate(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for resolved private IP, got %d: %s",
					w.Code, w.Body.String())
			}
		})
	}
}

func TestSecurityParity_Navigate_TrustedResolveCIDRBypassesPrivateIP(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	for _, p := range navigateProviders() {
		t.Run(p.Name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{
				TrustedResolveCIDRs: []string{"10.0.0.0/8"},
			}
			h := p.Setup(t, cfg)

			req := httptest.NewRequest("POST", "/navigate",
				strings.NewReader(`{"url":"https://internal.example.com"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleNavigate(w, req)

			// Should NOT get 403 — the trusted CIDR should allow the resolved
			// private IP. The request may fail later (500 from Chrome mock)
			// but must not be blocked by the SSRF guard.
			if w.Code == http.StatusForbidden {
				t.Fatalf("trusted CIDR should allow private IP, got 403: %s",
					w.Body.String())
			}
		})
	}
}

func TestSecurityParity_Navigate_DomainCheckRunsBeforeDNS(t *testing.T) {
	// Verify that the domain allowlist check fires before DNS resolution
	// for every provider.
	old := netguard.ResolveHostIPs
	t.Cleanup(func() { netguard.ResolveHostIPs = old })

	for _, p := range navigateProviders() {
		t.Run(p.Name, func(t *testing.T) {
			resolveCalled := false
			netguard.ResolveHostIPs = func(context.Context, string, string) ([]net.IP, error) {
				resolveCalled = true
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			}

			cfg := &config.RuntimeConfig{
				AllowedDomains: []string{"safe.example.com"},
				IDPI: config.IDPIConfig{
					Enabled:    true,
					StrictMode: true,
				},
			}
			h := p.Setup(t, cfg)

			req := httptest.NewRequest("POST", "/navigate",
				strings.NewReader(`{"url":"https://blocked.example.net"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleNavigate(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
			}
			if resolveCalled {
				t.Fatal("DNS resolver was called even though domain should have been blocked first")
			}
		})
	}
}
