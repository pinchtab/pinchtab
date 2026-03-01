package handlers

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleHealth_Response(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.HandleHealth(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
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

func TestHandleTabNavigate_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//navigate", bytes.NewReader([]byte(`{"url":"https://example.com"}`)))
	w := httptest.NewRecorder()
	h.HandleTabNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabNavigate_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/navigate", bytes.NewReader([]byte(`{"tabId":"tab_other","url":"https://example.com"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEvaluate_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/evaluate", bytes.NewReader([]byte(`{"tabId":"tab_other","expression":"1+1"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
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

func TestHandleAction_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleShutdown_CallsFunc(t *testing.T) {
	called := make(chan bool, 1)
	doShutdown := func() { called <- true }
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	handler := h.HandleShutdown(doShutdown)
	req := httptest.NewRequest("POST", "/shutdown", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	select {
	case <-called:
	case <-time.After(500 * time.Millisecond):
		t.Error("expected doShutdown to be called within 500ms")
	}
}
