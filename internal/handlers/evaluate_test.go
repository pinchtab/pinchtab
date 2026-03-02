package handlers

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

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
