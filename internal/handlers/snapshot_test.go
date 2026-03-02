package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleSnapshot_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSnapshot_WithFilter(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?filter=interactive&format=compact", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSnapshot_FileOutput_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{StateDir: "/tmp"}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?output=file", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSnapshot_PathTraversal(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{StateDir: "/tmp/pinchtab-test"}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?output=file&path=../../etc/passwd", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	// Should fail — either 404 (no tab) or 400 (path traversal)
	if w.Code == http.StatusOK {
		t.Error("expected error for path traversal")
	}
}

func TestHandleTabSnapshot_MissingTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//snapshot", nil)
	w := httptest.NewRecorder()
	h.HandleTabSnapshot(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabSnapshot_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_123/snapshot", nil)
	req.SetPathValue("id", "tab_123")
	w := httptest.NewRecorder()
	h.HandleTabSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
