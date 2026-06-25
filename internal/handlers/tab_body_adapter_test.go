package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// captureRoot returns an http.HandlerFunc that decodes the JSON body it
// receives into got and records its Content-Type header.
func captureRoot(t *testing.T, got *map[string]any, contentType *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		*contentType = r.Header.Get("Content-Type")
		body := map[string]any{}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil && err != io.EOF {
			t.Fatalf("root handler failed to decode forwarded body: %v", err)
		}
		*got = body
		w.WriteHeader(http.StatusOK)
	}
}

func TestWithPathTabIDBody_InjectsPathID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	var got map[string]any
	var contentType string

	req := httptest.NewRequest("POST", "/tabs/tab_abc/action", strings.NewReader(`{"ref":"e5"}`))
	req.SetPathValue("id", "tab_abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.withPathTabIDBody(w, req, captureRoot(t, &got, &contentType))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got["tabId"] != "tab_abc" {
		t.Fatalf("expected forwarded tabId = tab_abc, got %v", got["tabId"])
	}
	if got["ref"] != "e5" {
		t.Fatalf("expected forwarded ref preserved as e5, got %v", got["ref"])
	}
	if contentType != "application/json" {
		t.Fatalf("expected forwarded Content-Type application/json, got %q", contentType)
	}
}

func TestWithPathTabIDBody_EmptyBodyTolerated(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	var got map[string]any
	var contentType string

	// nil body -> EOF on decode; must be tolerated and still inject tabId.
	req := httptest.NewRequest("POST", "/tabs/tab_abc/action", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()

	h.withPathTabIDBody(w, req, captureRoot(t, &got, &contentType))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty body, got %d: %s", w.Code, w.Body.String())
	}
	if got["tabId"] != "tab_abc" {
		t.Fatalf("expected injected tabId = tab_abc on empty body, got %v", got["tabId"])
	}
}

func TestWithPathTabIDBody_MatchingBodyTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	var got map[string]any
	var contentType string

	req := httptest.NewRequest("POST", "/tabs/tab_abc/action", strings.NewReader(`{"tabId":"tab_abc"}`))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()

	h.withPathTabIDBody(w, req, captureRoot(t, &got, &contentType))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching tabId, got %d: %s", w.Code, w.Body.String())
	}
	if got["tabId"] != "tab_abc" {
		t.Fatalf("expected forwarded tabId = tab_abc, got %v", got["tabId"])
	}
}

func TestWithPathTabIDBody_MissingPathID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	called := false
	root := func(w http.ResponseWriter, r *http.Request) { called = true }

	// No SetPathValue("id", ...) -> path id empty -> must 400 without calling root.
	req := httptest.NewRequest("POST", "/tabs//action", strings.NewReader(`{"ref":"e5"}`))
	w := httptest.NewRecorder()

	h.withPathTabIDBody(w, req, root)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing path id, got %d: %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("root handler should not be called when path id is missing")
	}
}

func TestWithPathTabIDBody_MismatchedBodyTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	called := false
	root := func(w http.ResponseWriter, r *http.Request) { called = true }

	req := httptest.NewRequest("POST", "/tabs/tab_abc/action", strings.NewReader(`{"tabId":"tab_other"}`))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()

	h.withPathTabIDBody(w, req, root)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched tabId, got %d: %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("root handler should not be called when body tabId mismatches path id")
	}
}
