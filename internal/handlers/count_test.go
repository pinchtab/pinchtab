package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleCount_MissingSelector(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/count", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "selector") {
		t.Fatalf("expected error about selector, got %s", w.Body.String())
	}
}

func TestHandleCount_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/count?selector=button", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCount_ValidCount(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		// Verify the expression is safely constructed
		if !strings.Contains(expression, "document.querySelectorAll") {
			t.Fatalf("expected querySelectorAll expression, got %s", expression)
		}
		if ptr, ok := out.(*int); ok {
			*ptr = 5
		}
		return nil
	}

	req := httptest.NewRequest("GET", "/count?selector=button.submit", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp countResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Selector != "button.submit" {
		t.Fatalf("expected selector button.submit, got %s", resp.Selector)
	}
	if resp.Count != 5 {
		t.Fatalf("expected count 5, got %d", resp.Count)
	}
}

func TestHandleTabCount_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//count?selector=button", nil)
	w := httptest.NewRecorder()
	h.HandleTabCount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabCount_ForwardsTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/count?selector=button", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabCount(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCountRoutesRegistered(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	paths := []string{"/count?selector=button", "/tabs/tab1/count?selector=button"}
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
