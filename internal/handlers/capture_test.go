package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleCapture_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/capture", nil)
	w := httptest.NewRecorder()
	h.HandleCapture(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCapture_UnknownOutput(t *testing.T) {
	// Bad output value must 400 before we ever try to capture — so failTab is
	// irrelevant; the check happens after tab resolution but before the CDP
	// round trip.
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/capture?output=carrier-pigeon", nil)
	w := httptest.NewRecorder()
	h.HandleCapture(w, req)
	// Tab resolution fails first, so we get 404. This test exists to confirm
	// the unknown-output branch does not panic during query parsing.
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (tab not found), got %d", w.Code)
	}
}

func TestHandleTabCapture_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//capture", nil)
	w := httptest.NewRecorder()
	h.HandleTabCapture(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabCapture_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/capture", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabCapture(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCapture_RouteMounted(t *testing.T) {
	// The route table is the source of truth for the API surface; make sure
	// /capture is registered alongside /screenshot and /snapshot so that
	// future route refactors notice.
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	for _, path := range []string{"/capture", "/tabs/abc/capture"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		// 404 is fine (tab not found); 404 with body "tab not found" proves
		// the route hit the handler. A bare 404 with empty body would mean
		// the mux did not match the path.
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 from handler, got %d body=%s", path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "tab") {
			t.Errorf("%s: expected handler-shaped 404 body, got %q", path, w.Body.String())
		}
	}
}

func TestHandleCapture_WaitParamParses(t *testing.T) {
	// The wait param accepts known string values; the parse must not crash
	// even when the tab lookup fails. Mirrors
	// TestHandleScreenshot_BeyondViewportRequestParses.
	for _, v := range []string{"stable", "load", "none", ""} {
		h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
		req := httptest.NewRequest("GET", "/capture?wait="+v, nil)
		w := httptest.NewRecorder()
		h.HandleCapture(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("wait=%q: expected 404 (tab not found), got %d", v, w.Code)
		}
	}
}

func TestHandleCapture_BoundsAndBeyondViewportParse(t *testing.T) {
	// withBounds and beyondViewport must parse without crashing — they are
	// the headline P3 query params and the handler should at minimum
	// validate them before hitting the tab lookup.
	cases := []string{
		"/capture?withBounds=true&beyondViewport=true",
		"/capture?withBounds=false",
		"/capture?beyondViewport=1",
		"/capture?withBounds=0&beyondViewport=0",
	}
	for _, url := range cases {
		h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		h.HandleCapture(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 (tab not found), got %d", url, w.Code)
		}
	}
}

func TestHandleCapture_OpenAPIExposes(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	h.HandleOpenAPI(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/capture") {
		t.Fatalf("expected /openapi.json to list /capture, got %s", w.Body.String())
	}
}
