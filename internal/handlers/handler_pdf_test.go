package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandlePDF_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/pdf", nil)
	w := httptest.NewRecorder()
	h.HandlePDF(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlePDF_NoTab_WithTabId(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/pdf?tabId=nonexistent", nil)
	w := httptest.NewRecorder()
	h.HandlePDF(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
