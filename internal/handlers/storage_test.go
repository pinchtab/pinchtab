package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleStorage_StateExportGateDisabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowStateExport: false}, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		body   string
	}{
		{name: "get blocked", method: http.MethodGet},
		{name: "post blocked", method: http.MethodPost, body: `{"key":"k","value":"v","type":"local"}`},
		{name: "delete blocked", method: http.MethodDelete, body: `{"type":"all"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *httptest.ResponseRecorder
			if tc.body != "" {
				r := httptest.NewRequest(tc.method, "/storage", bytes.NewReader([]byte(tc.body)))
				w := httptest.NewRecorder()
				h.HandleStorage(w, r)
				req = w
			} else {
				r := httptest.NewRequest(tc.method, "/storage", nil)
				w := httptest.NewRecorder()
				h.HandleStorage(w, r)
				req = w
			}

			if req.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", req.Code, req.Body.String())
			}
		})
	}
}

func TestHandleStorage_StateExportGateEnabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowStateExport: true}, nil, nil, nil)
	h.evalJS = func(ctx context.Context, expr string, out *string) error {
		switch {
		case bytes.Contains([]byte(expr), []byte("setItem")):
			*out = `{"success":true,"origin":"http://example.com"}`
		case bytes.Contains([]byte(expr), []byte("clear")) || bytes.Contains([]byte(expr), []byte("removeItem")):
			*out = `{"success":true,"origin":"http://example.com"}`
		default:
			*out = `{"local":[],"session":[],"origin":"http://example.com"}`
		}
		return nil
	}

	tests := []struct {
		name   string
		method string
		body   string
	}{
		{name: "get allowed", method: http.MethodGet},
		{name: "post allowed", method: http.MethodPost, body: `{"key":"k","value":"v","type":"local"}`},
		{name: "delete allowed", method: http.MethodDelete, body: `{"type":"all"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var r *http.Request
			if tc.body != "" {
				r = httptest.NewRequest(tc.method, "/storage", bytes.NewReader([]byte(tc.body)))
			} else {
				r = httptest.NewRequest(tc.method, "/storage", nil)
			}
			w := httptest.NewRecorder()
			h.HandleStorage(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestEndpointSecurityState_IncludesStorageMutations(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowStateExport: false}, nil, nil, nil)
	state := h.endpointSecurityStates()["stateExport"]

	want := []string{"GET /storage", "POST /storage", "DELETE /storage"}
	for _, p := range want {
		found := false
		for _, have := range state.Paths {
			if have == p {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in stateExport paths, got %v", p, state.Paths)
		}
	}
}

func TestOpenAPI_StorageModeledOnceWithMethodMetadata(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowStateExport: false}, nil, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)

	h.HandleOpenAPI(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if strings.Contains(w.Body.String(), "/storage (GET)") {
		t.Fatalf("openapi should not expose synthetic /storage (GET) path")
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal openapi: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths missing or invalid")
	}
	storagePath, ok := paths["/storage"].(map[string]any)
	if !ok {
		t.Fatalf("/storage path missing")
	}
	for _, method := range []string{"get", "post", "delete"} {
		meta, ok := storagePath[method].(map[string]any)
		if !ok {
			t.Fatalf("/storage.%s missing", method)
		}
		if _, ok := meta["x-pinchtab-enabled"]; !ok {
			t.Fatalf("/storage.%s missing x-pinchtab-enabled", method)
		}
	}
}
