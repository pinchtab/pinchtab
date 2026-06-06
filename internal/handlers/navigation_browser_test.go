package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
)

func TestHandleNavigate_BrowserParam_Valid(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "cloak"},
	}, nil, nil, nil)

	body := []byte(`{"url":"https://example.com","browser":"cloak"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should not return 400 (browser validation passes).
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected browser param 'cloak' to be accepted, got 400 body=%s", w.Body.String())
	}

	// If 200, verify route metadata includes the requested browser.
	if w.Code == http.StatusOK {
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		route, ok := resp["route"].(map[string]any)
		if !ok {
			t.Fatalf("expected route in response, got %v", resp)
		}
		if got := route["requestedProvider"]; got != "cloak" {
			t.Fatalf("expected requestedProvider=cloak, got %v", got)
		}
	}
}

func TestHandleNavigate_EnsuresResolvedBrowserTargetConfig(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{
		DefaultBrowser:    config.BrowserChrome,
		BrowsersAvailable: []string{"chrome", "cloak"},
		ChromeBinary:      "/usr/bin/chrome",
		Targets: config.BrowserTargetsConfig{
			"cloak-local": {
				Provider: config.BrowserCloak,
				Binary:   "/opt/cloakbrowser/chrome",
			},
		},
	}, nil, nil, nil)

	body := []byte(`{"url":"about:blank","browser":"cloak"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if m.ensureChromeCall != 1 {
		t.Fatalf("expected startup to be ensured once, got %d", m.ensureChromeCall)
	}
	if m.ensureChromeCfg == nil {
		t.Fatal("expected startup config to be recorded")
	}
	if m.ensureChromeCfg.DefaultBrowser != config.BrowserCloak {
		t.Fatalf("DefaultBrowser = %q, want %q", m.ensureChromeCfg.DefaultBrowser, config.BrowserCloak)
	}
	if m.ensureChromeCfg.ChromeBinary != "/opt/cloakbrowser/chrome" {
		t.Fatalf("ChromeBinary = %q, want target binary", m.ensureChromeCfg.ChromeBinary)
	}
}

func TestHandleNavigate_BrowserParam_Invalid_Returns400(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	body := []byte(`{"url":"https://example.com","browser":"invalid"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid browser, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' in error, got %s", w.Body.String())
	}
}

func TestHandleNavigate_BrowserParam_GET(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "cloak"},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/navigate?url=https://example.com&browser=cloak", nil)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should not return 400.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected browser param 'cloak' to be accepted via GET, got 400 body=%s", w.Body.String())
	}
}

func TestHandleSnapshot_BrowserParam_Valid(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "cloak"},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/snapshot?browser=cloak", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)

	// Should not return 400 (validation passes).
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected browser param 'cloak' to be accepted, got 400 body=%s", w.Body.String())
	}
}

func TestHandleSnapshot_BrowserParam_Invalid_Returns400(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/snapshot?browser=invalid", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid browser, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' in error, got %s", w.Body.String())
	}
}

func TestHandleCapture_BrowserParam_Invalid_Returns400(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/capture?browser=invalid", nil)
	w := httptest.NewRecorder()
	h.HandleCapture(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid browser, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' in error, got %s", w.Body.String())
	}
}

func TestHandleAction_BrowserParam_Invalid_Returns400(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	body := []byte(`{"kind":"click","ref":"e1","browser":"invalid"}`)
	req := httptest.NewRequest("POST", "/action", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid browser, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' in error, got %s", w.Body.String())
	}
}

func TestHandleText_BrowserParam_Invalid_Returns400(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/text?browser=invalid", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid browser, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' in error, got %s", w.Body.String())
	}
}

// --- Browser selection precedence tests ---
//
// These tests verify the handler-level browser resolution chain:
//   request param > session browser > instance browser > global default > chrome
//
// Strategy: when the resolved browser is not "chrome" and is neither in
// BrowsersAvailable nor the global browser registry, the handler returns
// 400 "unknown browser". We use a fake name ("fake-browser") for this
// purpose to prove which source the handler picked. When the resolved
// browser IS "chrome", validation is skipped (it's always accepted).
//
// Note: "cloak" is registered in the global browser registry via a
// transitive import (bridge → bridge/runtime → browsers/cloak), so it
// passes ParseBrowser validation even when absent from BrowsersAvailable.
// We use "fake-browser" (not in the registry) wherever we need a
// guaranteed 400 to prove a precedence level was reached.

func TestHandleNavigate_SessionBrowser_AppliedWhenNoRequestParam(t *testing.T) {
	// Session has browser "fake-browser", which is NOT in BrowsersAvailable
	// and NOT in the global registry. With no request browser param,
	// ResolveBrowser should pick session's "fake-browser", which fails
	// ParseBrowser validation → 400.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	sess := &session.Session{Browser: "fake-browser"}
	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 because session browser 'fake-browser' is not available, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' error from session browser, got %s", w.Body.String())
	}
}

func TestHandleNavigate_RequestBrowser_OverridesSessionBrowser(t *testing.T) {
	// Session has "fake-browser" (would cause 400 if used), but request
	// explicitly sets browser="chrome". Request should win, and "chrome"
	// always passes validation.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	sess := &session.Session{Browser: "fake-browser"}
	body := []byte(`{"url":"https://example.com","browser":"chrome"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should NOT get 400 — request "chrome" wins over session "fake-browser".
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected request browser 'chrome' to override session 'fake-browser', but got 400 body=%s", w.Body.String())
	}
}

func TestHandleNavigate_GlobalDefault_AppliedWhenNoRequestOrSession(t *testing.T) {
	// DefaultBrowser is "fake-browser" which is NOT in BrowsersAvailable
	// or the registry. No request param, no session. ResolveBrowser should
	// pick global default "fake-browser", which fails validation → 400.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultBrowser:    "fake-browser",
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 because global default 'fake-browser' is not available, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' error from global default, got %s", w.Body.String())
	}
}

func TestHandleNavigate_EmptyConfig_DefaultsToChrome(t *testing.T) {
	// No DefaultBrowser, no session, no request param.
	// ResolveBrowser returns "chrome", which always passes validation.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should NOT get 400 — chrome is always accepted.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected empty config to default to chrome (no 400), got body=%s", w.Body.String())
	}
}

func TestHandleNavigate_RequestConflictsWithSession_RequestWins(t *testing.T) {
	// Session browser = "cloak", request browser = "chrome".
	// Both are valid browsers. Verify request wins by checking
	// route metadata in the response.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "cloak"},
	}, nil, nil, nil)

	sess := &session.Session{Browser: "cloak"}
	body := []byte(`{"url":"https://example.com","browser":"chrome"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should not be 400 — "chrome" is valid.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected request browser 'chrome' to be accepted, got 400 body=%s", w.Body.String())
	}

	// If we get a 200, verify route metadata reflects the request browser.
	if w.Code == http.StatusOK {
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		route, ok := resp["route"].(map[string]any)
		if !ok {
			t.Fatalf("expected route in response, got %v", resp)
		}
		if got := route["requestedProvider"]; got != "chrome" {
			t.Errorf("expected requestedProvider=chrome (request wins over session), got %v", got)
		}
	}
}

func TestHandleNavigate_SessionBrowser_AcceptedWhenAvailable(t *testing.T) {
	// Session browser = "cloak", "cloak" IS in BrowsersAvailable.
	// No request param. Should pass validation (not 400).
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "cloak"},
	}, nil, nil, nil)

	sess := &session.Session{Browser: "cloak"}
	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should NOT get 400 — session browser "cloak" is available.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected session browser 'cloak' to be accepted when available, got 400 body=%s", w.Body.String())
	}
}

func TestHandleNavigate_GlobalDefault_AcceptedWhenAvailable(t *testing.T) {
	// DefaultBrowser = "cloak", "cloak" IS in BrowsersAvailable.
	// No request param, no session. Should pass validation (not 400).
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultBrowser:    "cloak",
		BrowsersAvailable: []string{"chrome", "cloak"},
	}, nil, nil, nil)

	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should NOT get 400 — global default "cloak" is available.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected global default 'cloak' to be accepted when available, got 400 body=%s", w.Body.String())
	}
}

func TestHandleNavigate_SessionOverridesGlobalDefault(t *testing.T) {
	// Global default = "chrome", session = "fake-browser".
	// "fake-browser" NOT in available list or registry.
	// If session wins, we get 400. If global default wins, we wouldn't
	// (since "chrome" is always accepted).
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultBrowser:    "chrome",
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	sess := &session.Session{Browser: "fake-browser"}
	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should get 400 — session "fake-browser" wins over global default "chrome".
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 because session 'fake-browser' overrides global default 'chrome', got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("expected 'unknown browser' error, got %s", w.Body.String())
	}
}

func TestHandleNavigate_RequestOverridesSessionAndDefault(t *testing.T) {
	// Session = "fake-browser" (would cause 400), global default = "fake-browser"
	// (would also cause 400), but request = "chrome". Request should win.
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultBrowser:    "fake-browser",
		BrowsersAvailable: []string{"chrome"},
	}, nil, nil, nil)

	sess := &session.Session{Browser: "fake-browser"}
	body := []byte(`{"url":"https://example.com","browser":"chrome"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	// Should NOT get 400 — request "chrome" wins over both session and default.
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected request 'chrome' to override session and default 'fake-browser', but got 400 body=%s", w.Body.String())
	}
}
