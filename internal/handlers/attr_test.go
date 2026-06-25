package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleGetAttr_MissingRef(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/attr?name=href", nil)
	w := httptest.NewRecorder()
	h.HandleGetAttr(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ref") {
		t.Fatalf("expected error about ref, got %s", w.Body.String())
	}
}

func TestHandleGetAttr_MissingName(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/attr?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleGetAttr(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "name") {
		t.Fatalf("expected error about name, got %s", w.Body.String())
	}
}

func TestHandleGetAttr_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/attr?ref=e5&name=href", nil)
	w := httptest.NewRecorder()
	h.HandleGetAttr(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetAttr_NoSnapshotCache(t *testing.T) {
	mb := &attrMockBridge{refCache: nil}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/attr?ref=e5&name=href", nil)
	w := httptest.NewRecorder()
	h.HandleGetAttr(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Fatalf("expected not-found error, got %s", w.Body.String())
	}
}

func TestHandleGetAttr_RefNotFound(t *testing.T) {
	mb := &attrMockBridge{
		refCache: &bridge.RefCache{
			Refs:    map[string]int64{"e1": 100},
			Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/attr?ref=e99&name=href", nil)
	w := httptest.NewRecorder()
	h.HandleGetAttr(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "e99") {
		t.Fatalf("expected not-found error mentioning e99, got %s", w.Body.String())
	}
}

func TestHandleTabGetAttr_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//attr?ref=e5&name=href", nil)
	w := httptest.NewRecorder()
	h.HandleTabGetAttr(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabGetAttr_ForwardsTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/attr?ref=e5&name=href", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabGetAttr(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAttrRoutesRegistered(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	paths := []string{"/attr?ref=e1&name=href", "/tabs/tab1/attr?ref=e1&name=href"}
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

// attrMockBridge extends mockBridge with GetRefCache support.
type attrMockBridge struct {
	mockBridge
	refCache *bridge.RefCache
}

func (m *attrMockBridge) GetRefCache(tabID string) *bridge.RefCache {
	return m.refCache
}
