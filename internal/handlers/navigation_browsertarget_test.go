package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// fakeOrchestrator stubs bridge.OrchestratorService; only FindInstanceByTab
// is wired, the rest are no-ops.
type fakeOrchestrator struct {
	tabs   map[string]*bridge.Instance
	lookup int
}

func (f *fakeOrchestrator) RegisterHandlers(mux *http.ServeMux) {}
func (f *fakeOrchestrator) Launch(name, port string, headless bool, extensionPaths []string) (*bridge.Instance, error) {
	return nil, nil
}
func (f *fakeOrchestrator) Stop(id string) error           { return nil }
func (f *fakeOrchestrator) StopProfile(name string) error  { return nil }
func (f *fakeOrchestrator) List() []bridge.Instance        { return nil }
func (f *fakeOrchestrator) Logs(id string) (string, error) { return "", nil }
func (f *fakeOrchestrator) FirstRunningURL() string        { return "" }
func (f *fakeOrchestrator) AllTabs() []bridge.InstanceTab  { return nil }
func (f *fakeOrchestrator) FindInstanceByTab(tabID string) (*bridge.Instance, bool) {
	f.lookup++
	inst, ok := f.tabs[tabID]
	if !ok {
		return nil, false
	}
	return inst, true
}
func (f *fakeOrchestrator) ScreencastURL(instanceID, tabID string) string { return "" }
func (f *fakeOrchestrator) Shutdown()                                     {}
func (f *fakeOrchestrator) ForceShutdown()                                {}

func newOrchWithTab(tabID string, inst *bridge.Instance) *fakeOrchestrator {
	return &fakeOrchestrator{tabs: map[string]*bridge.Instance{tabID: inst}}
}

// TestHandleNavigate_BrowserTargetIgnored verifies that sending a browserTarget
// field in the JSON body is silently ignored now that the field has been removed
// from the public NavigateRequest.
func TestHandleNavigate_BrowserTargetIgnored(t *testing.T) {
	orch := newOrchWithTab("tab1", &bridge.Instance{ID: "inst1", Browser: config.BrowserChrome})
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultTarget: "chrome-default",
		Targets: config.BrowserTargetsConfig{
			"chrome-default": {Provider: config.BrowserChrome},
			"cloak-default":  {Provider: config.BrowserCloak, Binary: "/opt/cloakbrowser/chrome"},
		},
	}, nil, nil, orch)

	// Even with a conflicting browserTarget in the body, no 409 should occur
	// because browserTarget is no longer parsed from the request.
	body := []byte(`{"tabId":"tab1","url":"https://example.com","browserTarget":"cloak-default"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("expected browserTarget to be ignored (no 409), got 409 body=%s", w.Body.String())
	}
}

// A plain tabId navigate against a tab owned by a non-default-browser instance
// must resolve to the instance's browser instead of 409ing (H5 regression).
func TestHandleNavigate_InstanceBrowserResolvedFromTab(t *testing.T) {
	orch := newOrchWithTab("tab1", &bridge.Instance{ID: "inst1", Browser: config.BrowserCloak})
	m := &mockBridge{navigateResult: &bridge.NavigateResult{
		TabID: "tab1",
		URL:   "http://localhost:3000/",
		Title: "Cloak Page",
	}}
	h := New(m, &config.RuntimeConfig{
		DefaultBrowser:    config.BrowserChrome,
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, orch)

	body := []byte(`{"tabId":"tab1","url":"http://localhost:3000"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("plain tabId navigate 409ed against its own instance browser: %s", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// An explicit browser param that disagrees with the owning instance still 409s,
// and the payload no longer duplicates instanceBrowser as instanceProvider.
func TestHandleNavigate_ExplicitBrowserConflictStill409(t *testing.T) {
	orch := newOrchWithTab("tab1", &bridge.Instance{ID: "inst1", Browser: config.BrowserCloak})
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultBrowser:    config.BrowserChrome,
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, orch)

	body := []byte(`{"tabId":"tab1","url":"http://localhost:3000","browser":"chrome"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for explicit conflicting browser, got %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("instanceBrowser")) {
		t.Fatalf("409 payload missing instanceBrowser: %s", w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("instanceProvider")) {
		t.Fatalf("409 payload still contains duplicate instanceProvider key: %s", w.Body.String())
	}
}
