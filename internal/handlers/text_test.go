package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleText_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/text", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleText_WithTabId(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/text?tabId=nonexistent", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleText_RawMode(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/text?mode=raw", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTabText_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//text", nil)
	w := httptest.NewRecorder()
	h.HandleTabText(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabText_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/text", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
