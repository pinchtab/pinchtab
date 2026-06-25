package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleGetBox_MissingRef(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/box", nil)
	w := httptest.NewRecorder()
	h.HandleGetBox(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ref") {
		t.Fatalf("expected error about ref, got %s", w.Body.String())
	}
}

func TestHandleGetBox_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/box?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleGetBox(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetBox_NoSnapshotCache(t *testing.T) {
	mb := &boxMockBridge{refCache: nil}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/box?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleGetBox(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Fatalf("expected not-found error, got %s", w.Body.String())
	}
}

func TestHandleGetBox_RefNotFound(t *testing.T) {
	mb := &boxMockBridge{
		refCache: &bridge.RefCache{
			Refs:    map[string]int64{"e1": 100},
			Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/box?ref=e99", nil)
	w := httptest.NewRecorder()
	h.HandleGetBox(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "e99") {
		t.Fatalf("expected not-found error mentioning e99, got %s", w.Body.String())
	}
}

func TestHandleTabGetBox_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//box?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleTabGetBox(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabGetBox_ForwardsTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/box?ref=e5", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabGetBox(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBoxRoutesRegistered(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	paths := []string{"/box?ref=e1", "/tabs/tab1/box?ref=e1"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), "404 page not found") {
				t.Fatalf("route %s not registered", path)
			}
		})
	}
}

// boxMockBridge extends mockBridge with GetRefCache support.
type boxMockBridge struct {
	mockBridge
	refCache *bridge.RefCache
}

func (m *boxMockBridge) GetRefCache(tabID string) *bridge.RefCache {
	return m.refCache
}
