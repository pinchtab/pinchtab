package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleTitle_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/title", nil)
	w := httptest.NewRecorder()
	h.HandleTitle(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleURL_WithTabID_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/url?tabId=missing", nil)
	w := httptest.NewRecorder()
	h.HandleURL(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleHTML_WithFrame_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/html?frameId=FRAME123&maxChars=50", nil)
	w := httptest.NewRecorder()
	h.HandleHTML(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleStyles_WithSelector_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/styles?selector=button&prop=display", nil)
	w := httptest.NewRecorder()
	h.HandleStyles(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleStyles_WithoutSelector_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/styles?prop=display", nil)
	w := httptest.NewRecorder()
	h.HandleStyles(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleTabInspect_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	cases := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{name: "title", fn: h.HandleTabTitle},
		{name: "url", fn: h.HandleTabURL},
		{name: "html", fn: h.HandleTabHTML},
		{name: "styles", fn: h.HandleTabStyles},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/tabs//"+tc.name, nil)
			w := httptest.NewRecorder()
			tc.fn(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleTabInspect_ForwardsTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)

	cases := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{name: "title", fn: h.HandleTabTitle},
		{name: "url", fn: h.HandleTabURL},
		{name: "html", fn: h.HandleTabHTML},
		{name: "styles", fn: h.HandleTabStyles},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/tabs/tab_abc/"+tc.name+"?selector=button", nil)
			req.SetPathValue("id", "tab_abc")
			w := httptest.NewRecorder()
			tc.fn(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", w.Code)
			}
		})
	}
}

func TestInspectRoutesRegistered(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	paths := []string{"/title", "/url", "/html", "/styles", "/tabs/tab1/title", "/tabs/tab1/url", "/tabs/tab1/html", "/tabs/tab1/styles"}
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
