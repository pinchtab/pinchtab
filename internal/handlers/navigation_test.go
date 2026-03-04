package handlers

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

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

func TestHandleTabDelete_MissingID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("DELETE", "/tabs/", nil)
	w := httptest.NewRecorder()
	h.HandleTabDelete(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400 for missing tab id, got %d", w.Code)
	}
}

func TestHandleTabDelete_Success(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("DELETE", "/tabs/tab_abc123", nil)
	req.SetPathValue("id", "tab_abc123")
	w := httptest.NewRecorder()
	h.HandleTabDelete(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
