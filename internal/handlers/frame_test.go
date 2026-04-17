package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleFrameGetCurrentScope(t *testing.T) {
	mb := &mockBridge{
		frameScopes: map[string]bridge.FrameScope{
			"tab1": {
				FrameID:   "child",
				FrameURL:  "https://example.com/frame",
				FrameName: "payment-frame",
				OwnerRef:  "e3",
			},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/frame", nil)
	w := httptest.NewRecorder()
	h.HandleFrame(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"scoped":true`) {
		t.Fatalf("expected scoped response, got %s", body)
	}
	if !strings.Contains(body, `"frameId":"child"`) {
		t.Fatalf("expected child frame id, got %s", body)
	}
}

func TestHandleFramePostMainClearsScope(t *testing.T) {
	mb := &mockBridge{
		frameScopes: map[string]bridge.FrameScope{
			"tab1": {FrameID: "child"},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/frame", strings.NewReader(`{"target":"main"}`))
	w := httptest.NewRecorder()
	h.HandleFrame(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, ok := mb.GetFrameScope("tab1"); ok {
		t.Fatal("expected frame scope to be cleared")
	}
	if !strings.Contains(w.Body.String(), `"scoped":false`) {
		t.Fatalf("expected unscoped response, got %s", w.Body.String())
	}
}

func TestFrameScopeForOwnerNodeFallsBackToRefCacheTargets(t *testing.T) {
	cache := &bridge.RefCache{
		Targets: map[string]bridge.RefTarget{
			"e3": {
				BackendNodeID:  20,
				ChildFrameID:   "child-frame",
				ChildFrameURL:  "https://example.com/embed",
				ChildFrameName: "payment-frame",
			},
		},
	}
	frames := map[string]bridge.RawFrame{
		"child-frame": {
			ID:   "child-frame",
			URL:  "https://example.com/embed",
			Name: "payment-frame",
		},
	}

	scope, ok := frameScopeForOwnerNode(20, cache, frames, nil)
	if !ok {
		t.Fatal("expected frame scope from ref cache fallback")
	}
	if scope.FrameID != "child-frame" {
		t.Fatalf("frame id = %q, want %q", scope.FrameID, "child-frame")
	}
	if scope.OwnerRef != "e3" {
		t.Fatalf("owner ref = %q, want %q", scope.OwnerRef, "e3")
	}
}

func TestMatchFrameByElementMetaMatchesIframeSrc(t *testing.T) {
	frames := map[string]bridge.RawFrame{
		"root": {
			ID:   "root",
			URL:  "https://example.com",
			Name: "main",
		},
		"child-frame": {
			ID:   "child-frame",
			URL:  "https://example.com/embed",
			Name: "payment-frame",
		},
	}

	scope, ok, err := matchFrameByElementMeta(frames, "root", bridge.FrameElementMeta{
		TagName: "iframe",
		Src:     "https://example.com/embed",
	})
	if err != nil {
		t.Fatalf("matchFrameByElementMeta() error = %v", err)
	}
	if !ok {
		t.Fatal("expected iframe src to match a frame")
	}
	if scope.FrameID != "child-frame" {
		t.Fatalf("frame id = %q, want %q", scope.FrameID, "child-frame")
	}
}
