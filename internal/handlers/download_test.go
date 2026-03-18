package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type downloadPolicyBridge struct {
	bridge.BridgeAPI
	lock     *bridge.LockInfo
	policy   bridge.TabPolicyState
	hasState bool
}

func (m *downloadPolicyBridge) BrowserContext() context.Context {
	return context.Background()
}

func (m *downloadPolicyBridge) TabContext(tabID string) (context.Context, string, error) {
	ctx, _ := chromedp.NewContext(context.Background())
	return ctx, tabID, nil
}

func (m *downloadPolicyBridge) ListTargets() ([]*target.Info, error) {
	return []*target.Info{{TargetID: "tab1", Type: "page"}}, nil
}

func (m *downloadPolicyBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	return m.lock
}

func (m *downloadPolicyBridge) GetTabPolicyState(tabID string) (bridge.TabPolicyState, bool) {
	return m.policy, m.hasState
}

func TestHandleDownload_MissingURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDownload_EmptyURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?url=", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty URL, got %d", w.Code)
	}
}

func TestValidateDownloadURL(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "localhost":
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://pinchtab.com/file.pdf", false},
		{"valid http", "http://pinchtab.com/page", false},
		{"file scheme", "file:///etc/passwd", true},
		{"ftp scheme", "ftp://pinchtab.com/file", true},
		{"data scheme", "data:text/html,hello", true},
		{"localhost", "http://localhost:8080/secret", true},
		{"loopback ipv4", "http://127.0.0.1/secret", true},
		{"loopback ipv6", "http://[::1]/secret", true},
		{"private ipv4 literal", "http://192.168.1.10/secret", true},
		{"shared address space", "http://100.64.0.1/secret", true},
		{"benchmark network", "http://198.18.0.1/secret", true},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data", true},
		{"localhost suffix", "http://foo.localhost/secret", true},
		{"empty scheme", "://pinchtab.com", true},
		{"no scheme", "pinchtab.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDownloadURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDownloadRemoteIPAddress(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "empty allowed", raw: "", wantErr: false},
		{name: "public ipv4", raw: "93.184.216.34", wantErr: false},
		{name: "public ipv6", raw: "2606:2800:220:1:248:1893:25c8:1946", wantErr: false},
		{name: "bracketed ipv6", raw: "[::1]", wantErr: true},
		{name: "loopback ipv4", raw: "127.0.0.1", wantErr: true},
		{name: "private ipv4", raw: "192.168.1.10", wantErr: true},
		{name: "metadata ipv4", raw: "169.254.169.254", wantErr: true},
		{name: "garbage", raw: "not-an-ip", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadRemoteIPAddress(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDownloadRemoteIPAddress(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestHandleDownload_SSRFBlocked(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	urls := []string{
		"file:///etc/passwd",
		"http://localhost:8080",
		"http://127.0.0.1/admin",
		"http://192.168.1.10/admin",
		"http://100.64.0.1/admin",
		"http://198.18.0.1/admin",
		"http://169.254.169.254/latest/meta-data",
	}
	for _, u := range urls {
		req := httptest.NewRequest("GET", "/download?url="+u, nil)
		w := httptest.NewRecorder()
		h.HandleDownload(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for SSRF URL %q, got %d", u, w.Code)
		}
	}
}

func TestHandleTabDownload_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//download?url=https://pinchtab.com", nil)
	w := httptest.NewRecorder()
	h.HandleTabDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabDownload_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/download?url=https://pinchtab.com", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabDownload(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDownload_Disabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?url=https://pinchtab.com/file.txt", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403 when download disabled, got %d", w.Code)
	}
}

func TestDownloadRequestGuard_BlocksBrowserSideRequests(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	guard := newDownloadRequestGuard(newDownloadURLGuard(nil), -1)
	err := guard.Validate("http://127.0.0.1:1337/increment", false)
	if err == nil {
		t.Fatal("expected browser-side localhost request to be blocked")
	}
	if !strings.Contains(err.Error(), "unsafe browser request") {
		t.Fatalf("expected unsafe browser request error, got %v", err)
	}
}

func TestDownloadURLGuard_EnforcesAllowedDomains(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com", "cdn.pinchtab.com", "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	guard := newDownloadURLGuard([]string{"pinchtab.com", "*.pinchtab.com"})

	if err := guard.Validate("https://pinchtab.com/file.pdf"); err != nil {
		t.Fatalf("expected exact allowlist match to pass, got %v", err)
	}
	if err := guard.Validate("https://cdn.pinchtab.com/file.pdf"); err != nil {
		t.Fatalf("expected wildcard allowlist match to pass, got %v", err)
	}

	err := guard.Validate("https://example.com/file.pdf")
	if err == nil {
		t.Fatal("expected download allowlist to reject example.com")
	}
	if !strings.Contains(err.Error(), "security.downloadAllowedDomains") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestValidateTabScopedDownloadURL(t *testing.T) {
	tests := []struct {
		name       string
		currentURL string
		requestURL string
		wantErr    bool
	}{
		{"same origin", "https://pinchtab.com/app", "https://pinchtab.com/file.pdf", false},
		{"same origin with port", "http://127.0.0.1:9867/page", "http://127.0.0.1:9867/file", false},
		{"cross origin", "https://pinchtab.com/app", "https://example.com/file.pdf", true},
		{"non http current page", "about:blank", "https://pinchtab.com/file.pdf", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTabScopedDownloadURL(tt.currentURL, tt.requestURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateTabScopedDownloadURL(%q, %q) error = %v, wantErr %v", tt.currentURL, tt.requestURL, err, tt.wantErr)
			}
		})
	}
}

func TestParseContentLengthHeader(t *testing.T) {
	headers := network.Headers{
		"Content-Length": "12345",
	}
	size, ok := parseContentLengthHeader(headers)
	if !ok || size != 12345 {
		t.Fatalf("parseContentLengthHeader() = (%d, %v), want (12345, true)", size, ok)
	}
}

func TestHandleDownload_TabLocked(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	b := &downloadPolicyBridge{
		lock: &bridge.LockInfo{
			Owner:     "alice",
			ExpiresAt: time.Now().Add(time.Minute),
		},
	}
	h := New(b, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?tabId=tab1&url=https://pinchtab.com/file.txt", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423 for locked tab, got %d", w.Code)
	}
}

func TestHandleDownload_TabScopedCrossOriginBlockedForCookieAuth(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com", "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	b := &downloadPolicyBridge{
		hasState: true,
		policy: bridge.TabPolicyState{
			CurrentURL: "https://pinchtab.com/app",
			UpdatedAt:  time.Now(),
		},
	}
	h := New(b, &config.RuntimeConfig{
		AllowDownload: true,
		IDPI: config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: []string{"pinchtab.com"},
		},
	}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?tabId=tab1&url=https://example.com/file.txt", nil)
	req.AddCookie(&http.Cookie{Name: "pinchtab_auth_token", Value: "session"})
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin tab-scoped download, got %d", w.Code)
	}
}

func TestHandleDownload_TabScopedCrossOriginAllowedForHeaderAuth(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com", "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	b := &downloadPolicyBridge{
		hasState: true,
		policy: bridge.TabPolicyState{
			CurrentURL: "https://pinchtab.com/app",
			UpdatedAt:  time.Now(),
		},
	}
	h := New(b, &config.RuntimeConfig{
		AllowDownload: true,
		IDPI: config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: []string{"pinchtab.com"},
		},
	}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?tabId=tab1&url=https://example.com/file.txt", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code == http.StatusForbidden && strings.Contains(w.Body.String(), "download_scope_forbidden") {
		t.Fatalf("expected header-auth tab-scoped download to bypass same-origin scope check, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDownloadRequestGuard_TracksRedirectLimits(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	guard := newDownloadRequestGuard(newDownloadURLGuard(nil), 0)
	err := guard.Validate("https://pinchtab.com/redirected", true)
	if !errors.Is(err, bridge.ErrTooManyRedirects) {
		t.Fatalf("expected too-many-redirects error, got %v", err)
	}
}

func TestHandleDownload_RejectsURLOutsideAllowedDomains(t *testing.T) {
	originalResolver := resolveDownloadHostIPs
	t.Cleanup(func() {
		resolveDownloadHostIPs = originalResolver
	})
	resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		switch host {
		case "pinchtab.com", "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	h := New(&mockBridge{}, &config.RuntimeConfig{
		AllowDownload:          true,
		DownloadAllowedDomains: []string{"pinchtab.com"},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/download?url=https://example.com/file.txt", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for URL outside allowlist, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "downloadAllowedDomains") {
		t.Fatalf("expected allowlist error in response, got %s", w.Body.String())
	}
}
