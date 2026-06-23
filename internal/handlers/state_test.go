package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/state"
)

func TestHandleTabState(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	tabID := "tab1"
	req := httptest.NewRequest("GET", "/tabs/"+tabID+"/state", nil)
	req.SetPathValue("tabId", tabID)
	w := httptest.NewRecorder()

	h.HandleTabState(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["tabId"] != tabID {
		t.Fatalf("expected tabId %s, got %v", tabID, got["tabId"])
	}
	if got["dialogPresent"] != false {
		t.Fatalf("expected dialogPresent=false, got %v", got["dialogPresent"])
	}
}

// TestHandleStateLoadBatchesStorage verifies HandleStateLoad restores each origin's
// storage in a single Evaluate call (one per origin, not one per key) and that the
// per-origin item count returned by the in-page script propagates to the response.
func TestHandleStateLoadBatchesStorage(t *testing.T) {
	dir := t.TempDir()
	sf := &state.StateFile{
		Name:    "saved",
		Origins: []string{"https://a.example", "https://b.example"},
		Storage: map[string]state.OriginStorage{
			"https://a.example": {
				Local:   map[string]string{"k1": "v1", "k2": "v2"},
				Session: map[string]string{"s1": "sv1"},
			},
			"https://b.example": {
				Local:   map[string]string{"k3": "v3"},
				Session: map[string]string{"s2": "sv2", "s3": "sv3"},
			},
		},
	}
	if _, err := state.Save(dir, sf, ""); err != nil {
		t.Fatalf("save state: %v", err)
	}

	// The mock cannot run JS, so it stands in for the in-page script's return value
	// by counting the setItem call-sites in the bulk script (two per origin: the local
	// and session loops) and propagating that through the result pointer.
	mb := &mockBridge{
		evaluateFn: func(expression string, result any) error {
			if p, ok := result.(*int); ok {
				*p = strings.Count(expression, "setItem")
			}
			return nil
		},
	}
	h := New(mb, &config.RuntimeConfig{AllowStateExport: true, StateDir: dir}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/state/load", strings.NewReader(`{"name":"saved","tabId":"tab1"}`))
	w := httptest.NewRecorder()
	h.HandleStateLoad(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// One Evaluate per origin proves batching; the old per-key loop would have made
	// one call per stored key (6 total) instead of 2.
	if mb.evaluateCalls != len(sf.Storage) {
		t.Fatalf("expected %d Evaluate calls (one per origin), got %d", len(sf.Storage), mb.evaluateCalls)
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Each bulk script contains two setItem occurrences (local + session loops), so the
	// stub reports 2 per origin -> 4 across the two origins.
	if got["storageItemsRestored"] != float64(2*len(sf.Storage)) {
		t.Fatalf("expected storageItemsRestored=%d, got %v", 2*len(sf.Storage), got["storageItemsRestored"])
	}
	origins, ok := got["origins"].([]any)
	if !ok || len(origins) != len(sf.Origins) {
		t.Fatalf("expected %d origins, got %v", len(sf.Origins), got["origins"])
	}
}
