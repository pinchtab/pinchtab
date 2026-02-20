package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &StatusWriter{ResponseWriter: w, Code: 200}

	sw.WriteHeader(http.StatusNotFound)
	if sw.Code != http.StatusNotFound {
		t.Errorf("expected Code 404, got %d", sw.Code)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected recorded code 404, got %d", w.Code)
	}

	// Test default code
	w2 := httptest.NewRecorder()
	sw2 := &StatusWriter{ResponseWriter: w2, Code: 200}
	sw2.Write([]byte("ok"))
	if sw2.Code != 200 {
		t.Errorf("expected default code 200, got %d", sw2.Code)
	}
}

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"foo": "bar"}
	JSON(w, http.StatusCreated, data)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected content-type application/json, got %q", ct)
	}
	expectedBody := `{"foo":"bar"}` + "\n"
	if w.Body.String() != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, w.Body.String())
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	err := fmt.Errorf("bad request")
	Error(w, http.StatusBadRequest, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	expectedBody := `{"error":"bad request"}` + "\n"
	if w.Body.String() != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, w.Body.String())
	}
}
