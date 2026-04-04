package handlers

import (
	"bytes"
	"context"

	"github.com/pinchtab/pinchtab/internal/config"
	"net/http/httptest"
	"testing"
)

func TestHandleStorageDelete_EmptyBody(t *testing.T) {
	// Setup handlers with a mock bridge
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalJS = func(ctx context.Context, expr string, out *string) error {
		*out = `{"success":true,"origin":"http://example.com"}`
		return nil
	}

	// Create a DELETE request with NO body
	req := httptest.NewRequest("DELETE", "/storage", nil)
	w := httptest.NewRecorder()

	// Direct call to handleStorageDelete (which is unexported but in same package)
	h.handleStorageDelete(w, req)

	// It should NOT return 400 (EOF)
	// It might return 404 if the tab isn't found in the mock, but we want to see it pass the decode stage.
	if w.Code == 400 {
		t.Fatalf("expected handleStorageDelete to handle empty body, got 400: %s", w.Body.String())
	}

	// If it got past decode, it should try to find a tab.
	// Our mockBridge return a context for any tabID, so it should proceed to domain check.
	// We expect 200 if the mock allows it.
	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStorageDelete_WithBody(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalJS = func(ctx context.Context, expr string, out *string) error {
		*out = `{"success":true,"origin":"http://example.com"}`
		return nil
	}

	// Valid body
	body := `{"type": "local", "key": "test"}`
	req := httptest.NewRequest("DELETE", "/storage", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	h.handleStorageDelete(w, req)

	if w.Code != 200 {
		t.Fatalf("expected handleStorageDelete to handle valid body and return 200, got %d: %s", w.Code, w.Body.String())
	}
}
