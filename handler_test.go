package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestBridge creates a Bridge with initialized maps (no Chrome).
func newTestBridge() *Bridge {
	b := &Bridge{}
	b.TabManager = &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*refCache),
	}
	return b
}

// newTestBridgeWithTabs is an alias used across test files.
var newTestBridgeWithTabs = newTestBridge

func TestHandleNavigate_MissingURL(t *testing.T) {
	b := newTestBridge()
	body := `{"url": ""}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleNavigate_BadJSON(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleNavigate_NoTab(t *testing.T) {
	b := newTestBridge()
	body := `{"url": "https://example.com"}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleAction_UnknownKind(t *testing.T) {
	b := newTestBridge()

	body := `{"kind": "explode", "selector": "button"}`
	req := httptest.NewRequest("POST", "/action", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleAction(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleAction_RefNotFound_NoCache(t *testing.T) {
	b := newTestBridge()

	body := `{"kind": "click", "ref": "e99"}`
	req := httptest.NewRequest("POST", "/action", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleAction(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleAction_BadJSON(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("POST", "/action", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	b.handleAction(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleAction_NoTab(t *testing.T) {
	b := newTestBridge()
	body := `{"kind": "click", "ref": "e0"}`
	req := httptest.NewRequest("POST", "/action", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleAction(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleEvaluate_MissingExpression(t *testing.T) {
	b := newTestBridge()
	body := `{"expression": ""}`
	req := httptest.NewRequest("POST", "/evaluate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleEvaluate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEvaluate_BadJSON(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("POST", "/evaluate", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	b.handleEvaluate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEvaluate_NoTab(t *testing.T) {
	b := newTestBridge()
	body := `{"expression": "1+1"}`
	req := httptest.NewRequest("POST", "/evaluate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleEvaluate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTab_CloseNoTabID(t *testing.T) {
	b := newTestBridge()
	body := `{"action": "close"}`
	req := httptest.NewRequest("POST", "/tab", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleTab(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_BadAction(t *testing.T) {
	b := newTestBridge()
	body := `{"action": "destroy"}`
	req := httptest.NewRequest("POST", "/tab", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleTab(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_BadJSON(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("POST", "/tab", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	b.handleTab(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSnapshot_NoTab(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("GET", "/snapshot", nil)
	w := httptest.NewRecorder()
	b.handleSnapshot(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleText_NoTab(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("GET", "/text", nil)
	w := httptest.NewRecorder()
	b.handleText(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleScreenshot_NoTab(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("GET", "/screenshot", nil)
	w := httptest.NewRecorder()
	b.handleScreenshot(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSetCookies_BadJSON(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("POST", "/cookies", strings.NewReader("{broken"))
	w := httptest.NewRecorder()
	b.handleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for bad JSON, got %d", w.Code)
	}
}

func TestHandleSetCookies_MissingURL(t *testing.T) {
	b := newTestBridge()
	body := `{"cookies": [{"name": "test", "value": "123"}]}`
	req := httptest.NewRequest("POST", "/cookies", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for missing URL, got %d", w.Code)
	}
}

func TestHandleHealth_NoBrowser(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	b.handleHealth(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "disconnected") {
		t.Errorf("expected 'disconnected' in body, got %s", body)
	}
}

func TestHandleTabs_NoBrowser(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("GET", "/tabs", nil)
	w := httptest.NewRecorder()
	b.handleTabs(w, req)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleScreenshot_QualityParam(t *testing.T) {
	b := newTestBridge()

	req := httptest.NewRequest("GET", "/screenshot?quality=50", nil)
	w := httptest.NewRecorder()
	b.handleScreenshot(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 (no tab), got %d", w.Code)
	}
}

func TestHandleScreenshot_InvalidQuality(t *testing.T) {
	b := newTestBridge()
	req := httptest.NewRequest("GET", "/screenshot?quality=abc", nil)
	w := httptest.NewRecorder()
	b.handleScreenshot(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_InvalidURL(t *testing.T) {
	b := newTestBridge()
	body := `{"url": "not-a-url"}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 (no tab), got %d", w.Code)
	}
}

func TestHandleNavigate_WaitTitleClamp(t *testing.T) {

	b := newTestBridge()
	body := `{"url": "https://example.com", "waitTitle": 999}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 (no tab), got %d", w.Code)
	}
}

func TestHandleNavigate_NewTabNoChrome(t *testing.T) {
	b := newTestBridge()
	body := `{"url": "https://example.com", "newTab": true}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)

	if w.Code != 500 {
		t.Errorf("expected 500 (no browser), got %d", w.Code)
	}
}

func TestGetSetDeleteRefCache(t *testing.T) {
	b := newTestBridge()

	if c := b.GetRefCache("tab1"); c != nil {
		t.Error("expected nil for unknown tab")
	}

	b.SetRefCache("tab1", &refCache{refs: map[string]int64{"e0": 42}})
	c := b.GetRefCache("tab1")
	if c == nil || c.refs["e0"] != 42 {
		t.Error("expected cached ref e0=42")
	}

	b.DeleteRefCache("tab1")
	if c := b.GetRefCache("tab1"); c != nil {
		t.Error("expected nil after delete")
	}
}
