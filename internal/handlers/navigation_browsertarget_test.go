package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func decodeErrorCode(body string) string {
	var payload struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(body), &payload)
	return payload.Code
}

func TestHandleNavigate_BrowserTargetMatchingTab_TrimsAndResolves(t *testing.T) {
	orch := newOrchWithTab("tab1", &bridge.Instance{ID: "inst1", BrowserTarget: "chrome-default", BrowserProvider: "chrome"})
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultTarget: "chrome-default",
		Targets: config.BrowserTargetsConfig{
			"chrome-default": {Provider: config.BrowserProviderChrome},
			"cloak-default":  {Provider: config.BrowserProviderCloak, Binary: "/opt/cloakbrowser/chrome"},
		},
	}, nil, nil, orch)

	body := []byte(`{"tabId":"tab1","url":"https://example.com","browserTarget":" chrome-default "}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("expected matching target NOT to 409, got 409 body=%s", w.Body.String())
	}
	if orch.lookup != 1 {
		t.Fatalf("expected FindInstanceByTab to be called once, got %d", orch.lookup)
	}
}

func TestHandleNavigate_BrowserTargetConflict_409(t *testing.T) {
	orch := newOrchWithTab("tab1", &bridge.Instance{ID: "inst1", BrowserTarget: "chrome-default", BrowserProvider: "chrome"})
	h := New(&mockBridge{}, &config.RuntimeConfig{
		DefaultTarget: "chrome-default",
		Targets: config.BrowserTargetsConfig{
			"chrome-default": {Provider: config.BrowserProviderChrome},
			"cloak-default":  {Provider: config.BrowserProviderCloak, Binary: "/opt/cloakbrowser/chrome"},
		},
	}, nil, nil, orch)

	body := []byte(`{"tabId":"tab1","url":"https://example.com","browserTarget":" cloak-default "}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %d body=%s", w.Code, w.Body.String())
	}
	if code := decodeErrorCode(w.Body.String()); code != "browser_target_conflict" {
		t.Fatalf("expected error code browser_target_conflict, got %q body=%s", code, w.Body.String())
	}
}

func TestHandleNavigate_BrowserTargetUnknownTab_FallsThrough(t *testing.T) {
	orch := &fakeOrchestrator{tabs: map[string]*bridge.Instance{}}
	m := &mockBridge{failTab: true}
	h := New(m, &config.RuntimeConfig{}, nil, nil, orch)

	body := []byte(`{"tabId":"missing","url":"https://example.com","browserTarget":"cloak-default"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("expected unknown tab to fall through (not 409), got 409 body=%s", w.Body.String())
	}
	if orch.lookup != 1 {
		t.Fatalf("expected FindInstanceByTab to be consulted once, got %d", orch.lookup)
	}
	// Code shouldn't be browser_target_conflict regardless of downstream outcome.
	if strings.Contains(w.Body.String(), "browser_target_conflict") {
		t.Fatalf("unexpected browser_target_conflict in response body: %s", w.Body.String())
	}
}

func TestHandleNavigate_BrowserTargetLegacyInstance_NoConflict(t *testing.T) {
	// Legacy instance: BrowserTarget empty.
	orch := newOrchWithTab("tab1", &bridge.Instance{ID: "inst1"})
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, orch)

	body := []byte(`{"tabId":"tab1","url":"https://example.com","browserTarget":"cloak-default"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("expected legacy instance to skip conflict check, got 409 body=%s", w.Body.String())
	}
}

func TestHandleNavigate_BrowserTargetNoTabID_Accepted(t *testing.T) {
	orch := &fakeOrchestrator{tabs: map[string]*bridge.Instance{}}
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, orch)

	body := []byte(`{"url":"https://example.com","browserTarget":"cloak-default"}`)
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code == http.StatusConflict {
		t.Fatalf("expected no-tabId + target to be accepted, got 409 body=%s", w.Body.String())
	}
	if orch.lookup != 0 {
		t.Fatalf("expected FindInstanceByTab NOT to be called without tabId, got %d", orch.lookup)
	}
}
