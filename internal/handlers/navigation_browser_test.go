package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
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

	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected browser param 'cloak' to be accepted, got 400 body=%s", w.Body.String())
	}

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
		BrowserBinary:     "/usr/bin/chrome",
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

	if m.ensureBrowserCall != 1 {
		t.Fatalf("expected startup to be ensured once, got %d", m.ensureBrowserCall)
	}
	if m.ensureBrowserCfg == nil {
		t.Fatal("expected startup config to be recorded")
	}
	if m.ensureBrowserCfg.DefaultBrowser != config.BrowserCloak {
		t.Fatalf("DefaultBrowser = %q, want %q", m.ensureBrowserCfg.DefaultBrowser, config.BrowserCloak)
	}
	if m.ensureBrowserCfg.BrowserBinary != "/opt/cloakbrowser/chrome" {
		t.Fatalf("BrowserBinary = %q, want target binary", m.ensureBrowserCfg.BrowserBinary)
	}
}

// A failed navigate must not strand the blank tab created for the new-tab
// path: it never carried the requested URL and would only burn a MaxTabs
// slot until eviction.
func TestHandleNavigate_FailedNavigateClosesBlankTab(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	m := &mockBridge{
		navigateErr: fmt.Errorf("net::ERR_CONNECTION_REFUSED"),
	}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := []byte(`{"url":"https://example.com"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code < 400 {
		t.Fatalf("expected navigate failure status, got %d body=%s", w.Code, w.Body.String())
	}
	if len(m.createTabURLs) != 1 {
		t.Fatalf("expected one blank tab creation, got %v", m.createTabURLs)
	}
	if len(m.closedTabs) != 1 || m.closedTabs[0] != "tab_abc12345" {
		t.Fatalf("blank tab not closed on navigate failure; closedTabs = %v", m.closedTabs)
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

	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected request browser 'chrome' to be accepted, got 400 body=%s", w.Body.String())
	}

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

	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected request 'chrome' to override session and default 'fake-browser', but got 400 body=%s", w.Body.String())
	}
}

// The ghost-chrome adapter can serve a navigate from its own static tab; the
// handler must respond with the adapter's tab instead of the tab it drove.
func TestHandleNavigate_AdapterServedTab_ExistingTab_ReturnsAdapterResult(t *testing.T) {
	m := &mockBridge{navigateResult: &bridge.NavigateResult{
		TabID: "lite-1",
		URL:   "http://localhost:3000/",
		Title: "Static Page",
	}}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000","tabId":"tab1"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["tabId"] != "lite-1" {
		t.Fatalf("tabId = %v, want lite-1 (adapter-served tab)", resp["tabId"])
	}
	if resp["url"] != "http://localhost:3000/" {
		t.Fatalf("url = %v, want adapter URL", resp["url"])
	}
	if resp["title"] != "Static Page" {
		t.Fatalf("title = %v, want adapter title", resp["title"])
	}
}

func TestHandleNavigate_AdapterServedTab_NewTab_ClosesUnusedBlankTab(t *testing.T) {
	m := &mockBridge{navigateResult: &bridge.NavigateResult{
		TabID: "lite-1",
		URL:   "http://localhost:3000/",
		Title: "Static Page",
	}}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["tabId"] != "lite-1" {
		t.Fatalf("tabId = %v, want lite-1 (adapter-served tab)", resp["tabId"])
	}
	if len(m.createTabURLs) == 0 {
		t.Fatal("expected the blank Chrome tab to have been created before Navigate")
	}
	if len(m.closedTabs) != 1 || m.closedTabs[0] != "tab_abc12345" {
		t.Fatalf("closedTabs = %v, want the unused blank tab [tab_abc12345]", m.closedTabs)
	}
}

// When the adapter's result identifies the same tab the handler drove (chrome,
// cloak, escalated ghost-chrome), the normal response path is unchanged.
func TestHandleNavigate_SameTabResult_NormalPathUnchanged(t *testing.T) {
	m := &mockBridge{navigateResult: &bridge.NavigateResult{
		TabID: "tab_abc12345",
		URL:   "http://localhost:3000/",
		Title: "Chrome Page",
	}}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["tabId"] != "tab_abc12345" {
		t.Fatalf("tabId = %v, want tab_abc12345 (handler-driven tab)", resp["tabId"])
	}
	if len(m.closedTabs) != 0 {
		t.Fatalf("no tab should be closed on the same-tab path, closed %v", m.closedTabs)
	}
}

func TestHandleNavigate_ExplicitBrowserConflictsWithRunning_409(t *testing.T) {
	m := &mockBridge{runningBrowser: config.BrowserChrome}
	h := New(m, &config.RuntimeConfig{
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000","browser":"cloak"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("explicit cloak against running chrome should 409, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "runningBrowser") {
		t.Fatalf("409 payload missing runningBrowser: %s", w.Body.String())
	}
}

// An unknown browser name must fail validation (400) even when a different
// browser is running — a 409 advising "restart with --browser nonexistent"
// points the caller at a browser that doesn't exist.
func TestHandleNavigate_UnknownBrowserWithRunningBrowser_400Not409(t *testing.T) {
	m := &mockBridge{runningBrowser: config.BrowserGhostChrome}
	h := New(m, &config.RuntimeConfig{
		BrowsersAvailable: []string{config.BrowserGhostChrome},
	}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000","browser":"nonexistent_xyz"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown browser should 400 before the running-browser conflict check, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("400 payload should name the unknown browser error: %s", w.Body.String())
	}
}

func TestHandleNavigate_ImplicitBrowserIgnoresRunningMismatch(t *testing.T) {
	// Default chrome resolution against a running cloak server must not 409 —
	// only explicit intent conflicts.
	m := &mockBridge{
		runningBrowser: config.BrowserCloak,
		navigateResult: &bridge.NavigateResult{TabID: "tab_abc12345", URL: "http://localhost:3000/"},
	}
	h := New(m, &config.RuntimeConfig{
		DefaultBrowser:    config.BrowserChrome,
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("implicit resolution must not 409 against the running browser: %s", w.Body.String())
	}
}

func TestHandleNavigate_ExplicitBrowserMatchesRunning_OK(t *testing.T) {
	m := &mockBridge{
		runningBrowser: config.BrowserCloak,
		navigateResult: &bridge.NavigateResult{TabID: "tab_abc12345", URL: "http://localhost:3000/"},
	}
	h := New(m, &config.RuntimeConfig{
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000","browser":"cloak"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("matching explicit browser must not 409: %s", w.Body.String())
	}
}

func TestHandleNavigate_ExplicitBrowserNothingRunning_OK(t *testing.T) {
	// Before any browser is launched, an explicit param decides the launch.
	m := &mockBridge{
		navigateResult: &bridge.NavigateResult{TabID: "tab_abc12345", URL: "http://localhost:3000/"},
	}
	h := New(m, &config.RuntimeConfig{
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000","browser":"cloak"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("explicit browser with nothing running must not 409: %s", w.Body.String())
	}
}

// A static-accept on a fresh navigate must never launch Chrome.
func TestHandleNavigate_StaticFirstServesWithoutChromeLaunch(t *testing.T) {
	m := &mockBridge{
		staticFirstNavigate: true,
		navigateResult: &bridge.NavigateResult{
			TabID: "lite-1",
			URL:   "http://localhost:3000/",
			Title: "Static",
			Route: &browserops.RouteMetadata{RequestedBrowser: "ghost-chrome", UsedBrowser: "ghost-chrome"},
		},
	}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["tabId"] != "lite-1" {
		t.Fatalf("tabId = %v, want lite-1", resp["tabId"])
	}
	if m.ensureBrowserCall != 0 {
		t.Fatalf("Chrome was launched (%d ensure calls) for a static-served navigate", m.ensureBrowserCall)
	}
	if len(m.createTabURLs) != 0 {
		t.Fatalf("a Chrome tab was created for a static-served navigate: %v", m.createTabURLs)
	}
}

// On escalation the handler launches Chrome, retries with SkipStatic, and the
// route metadata reflects both attempts.
func TestHandleNavigate_StaticFirstEscalatesThroughHandler(t *testing.T) {
	m := &mockBridge{
		staticFirstNavigate: true,
		staticEscalate: &bridge.StaticEscalateError{
			Quality: 20,
			Reason:  "thin content",
			Route: &browserops.RouteMetadata{
				RequestedBrowser: "ghost-chrome",
				UsedBrowser:      "ghost-chrome",
				Quality:          20,
				Attempts: []browserops.RouteAttempt{
					{Browser: "ghost-chrome", Accepted: false, Reason: "thin content"},
				},
			},
		},
		navigateResult: &bridge.NavigateResult{TabID: "tab_abc12345", URL: "http://localhost:3000/"},
	}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	body := []byte(`{"url":"http://localhost:3000"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if m.ensureBrowserCall != 1 {
		t.Fatalf("ensure calls = %d, want 1 (escalation launches Chrome)", m.ensureBrowserCall)
	}
	if len(m.navigateParams) != 2 || !m.navigateParams[0].NoEscalate || !m.navigateParams[1].SkipStatic {
		t.Fatalf("expected NoEscalate then SkipStatic navigates, got %+v", m.navigateParams)
	}
	var resp struct {
		Route struct {
			Escalated bool `json:"escalated"`
			Attempts  []struct {
				Browser string `json:"browser"`
			} `json:"attempts"`
		} `json:"route"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Route.Escalated || len(resp.Route.Attempts) != 2 {
		t.Fatalf("route should show static + chrome attempts with escalated, got %s", w.Body.String())
	}
}
