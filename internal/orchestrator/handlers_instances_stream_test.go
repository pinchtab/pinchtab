package orchestrator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type noFlusherOrchestratorResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *noFlusherOrchestratorResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlusherOrchestratorResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *noFlusherOrchestratorResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func TestHandleLogsStreamByID_StreamingNotSupportedReturnsProblem(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	req := httptest.NewRequest(http.MethodGet, "/instances/inst-1/logs/stream", nil)
	w := &noFlusherOrchestratorResponseWriter{}

	o.handleLogsStreamByID(w, req)

	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type = %q, want application/problem+json", ct)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &payload); err != nil {
		t.Fatalf("decode problem payload: %v", err)
	}
	if payload["code"] != "streaming_not_supported" {
		t.Fatalf("code = %v, want streaming_not_supported", payload["code"])
	}
}

func TestHandleLogsStreamByID_StreamingDeadlineUnsupportedReturnsProblem(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	req := httptest.NewRequest(http.MethodGet, "/instances/inst-1/logs/stream", nil)
	w := httptest.NewRecorder()

	o.handleLogsStreamByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type = %q, want application/problem+json", ct)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode problem payload: %v", err)
	}
	if payload["code"] != "streaming_deadline_unsupported" {
		t.Fatalf("code = %v, want streaming_deadline_unsupported", payload["code"])
	}
}
