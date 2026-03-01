package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleDownload_MissingURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDownload_EmptyURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?url=", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty URL, got %d", w.Code)
	}
}

func TestHandleTabDownload_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//download?url=https://example.com", nil)
	w := httptest.NewRecorder()
	h.HandleTabDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabDownload_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/download?url=https://example.com", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabDownload(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
