package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

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
