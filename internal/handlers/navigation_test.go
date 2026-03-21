package handlers

import (
	"bytes"
	"context"
	"net"
	"net/http/httptest"
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
		target, err := validateNavigateTarget(rawURL, false)
		if err != nil {
			t.Fatalf("validateNavigateTarget(%q) error = %v", rawURL, err)
		}
		if target == nil || !target.allowInternal {
			t.Fatalf("validateNavigateTarget(%q) should allow local targets", rawURL)
		}
	}
}

func TestValidateNavigateTarget_RejectsPrivateLiteralIP(t *testing.T) {
	if _, err := validateNavigateTarget("http://192.168.1.10/app", false); err == nil {
		t.Fatal("validateNavigateTarget should reject private literal IPs")
	}
}

func TestValidateNavigateTarget_RejectsResolvedPrivateIP(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.10")}, nil
	})

	if _, err := validateNavigateTarget("https://example.com/app", false); err == nil {
		t.Fatal("validateNavigateTarget should reject hosts resolving to private IPs")
	}
}

func TestValidateNavigateTarget_AllowsResolvedPrivateIPWhenExplicitlyAllowlisted(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("172.18.0.5")}, nil
	})

	target, err := validateNavigateTarget("http://fixtures:80/app", true)
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
		IDPI: config.IDPIConfig{
			Enabled:        true,
			StrictMode:     true,
			AllowedDomains: []string{"fixtures"},
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
