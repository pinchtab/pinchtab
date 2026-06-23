package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleGetVisible_MissingRef(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/visible", nil)
	w := httptest.NewRecorder()
	h.HandleGetVisible(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ref") {
		t.Fatalf("expected error about ref, got %s", w.Body.String())
	}
}

func TestHandleGetVisible_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/visible?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleGetVisible(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetVisible_NoSnapshotCache(t *testing.T) {
	mb := &visibleMockBridge{refCache: nil}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/visible?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleGetVisible(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Fatalf("expected not-found error, got %s", w.Body.String())
	}
}

func TestHandleGetVisible_RefNotFound(t *testing.T) {
	mb := &visibleMockBridge{
		refCache: &bridge.RefCache{
			Refs:    map[string]int64{"e1": 100},
			Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/visible?ref=e99", nil)
	w := httptest.NewRecorder()
	h.HandleGetVisible(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "e99") {
		t.Fatalf("expected not-found error mentioning e99, got %s", w.Body.String())
	}
}

func TestHandleGetVisible_SelectorParamEquivalentToRef(t *testing.T) {
	newH := func() *Handlers {
		mb := &visibleMockBridge{
			refCache: &bridge.RefCache{
				Refs:    map[string]int64{"e1": 100},
				Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
			},
		}
		return New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	}

	// Both the unified-selector `selector` param and the legacy `ref` alias must
	// feed the same resolver: an unknown ref via either param yields the same 404.
	for _, q := range []string{"selector=e99", "ref=e99"} {
		req := httptest.NewRequest("GET", "/visible?"+q, nil)
		w := httptest.NewRecorder()
		newH().HandleGetVisible(w, req)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "e99") {
			t.Fatalf("%s: expected 404 mentioning e99, got %d: %s", q, w.Code, w.Body.String())
		}
	}

	// `selector` takes precedence over `ref` when both are present.
	req := httptest.NewRequest("GET", "/visible?selector=e99&ref=e1", nil)
	w := httptest.NewRecorder()
	newH().HandleGetVisible(w, req)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "e99") {
		t.Fatalf("expected selector precedence (404 mentioning e99), got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabGetVisible_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//visible?ref=e5", nil)
	w := httptest.NewRecorder()
	h.HandleTabGetVisible(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabGetVisible_ForwardsTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/visible?ref=e5", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabGetVisible(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVisibleRoutesRegistered(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	paths := []string{"/visible?ref=e1", "/tabs/tab1/visible?ref=e1"}
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

// visibleMockBridge extends mockBridge with GetRefCache support.
type visibleMockBridge struct {
	mockBridge
	refCache *bridge.RefCache
}

func (m *visibleMockBridge) GetRefCache(tabID string) *bridge.RefCache {
	return m.refCache
}
