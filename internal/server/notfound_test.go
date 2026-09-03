package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) (code, message string) {
	t.Helper()
	var resp struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not the JSON error envelope (%v): %s", err, w.Body.String())
	}
	return resp.Code, resp.Error
}

// An unrouted path used to escape the JSON contract as net/http's plain-text
// "404 page not found". Every other failure on the same server answers the
// envelope, so an unrouted one must too.
func TestUnroutedPathAnswersTheJSONEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "404 page not found") {
		t.Fatalf("still the bare mux 404, which breaks the JSON contract: %s", w.Body.String())
	}
	code, message := decodeEnvelope(t, w)
	if code != "not_found" {
		t.Errorf("code = %q, want not_found", code)
	}
	if message == "" {
		t.Error("the not-found refusal carries no message")
	}
}

// The generic fallback is a floor, not a replacement: a registered coded refusal
// still matches a pattern and must keep winning. The bridge-mode session family
// is the codebase's own example of a known-absent family answered with a code
// and a remedy, and it must not be flattened into the generic not_found body.
func TestCodedRefusalsWinOverTheGenericFallback(t *testing.T) {
	mux := http.NewServeMux()
	RegisterSessionsUnavailableInBridgeMode(mux)
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/sessions", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	code, _ := decodeEnvelope(t, w)
	if code != CodeSessionsUnavailableInBridgeMode {
		t.Fatalf("code = %q, want %q — the coded refusal was flattened into the generic fallback", code, CodeSessionsUnavailableInBridgeMode)
	}
}

// A matched route serves normally; the wrapper only fires when nothing is routed.
func TestMatchedRouteIsUntouched(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("live"))
	})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK || w.Body.String() != "live" {
		t.Fatalf("matched route not served: status=%d body=%q", w.Code, w.Body.String())
	}
}

// A request for a real path under an unregistered method is not "no such path":
// the wrapper preserves the 405 the mux would have written rather than reporting
// a 404, while still answering the JSON envelope.
func TestWrongMethodStaysA405Envelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/openapi.json", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if strings.Contains(w.Body.String(), "method not allowed\n") {
		t.Fatalf("still the bare mux 405 plain text: %s", w.Body.String())
	}
	code, _ := decodeEnvelope(t, w)
	if code != "method_not_allowed" {
		t.Errorf("code = %q, want method_not_allowed", code)
	}
}
