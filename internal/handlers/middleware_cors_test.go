package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func corsRoundTrip(t *testing.T, cfg *config.RuntimeConfig, method, origin string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	h := CorsMiddleware(config.NewLive(cfg), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "http://pinchtab:9867/health", nil)
	req.Host = "pinchtab:9867"
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w, reached
}

func TestCorsWithoutATokenAllowsAnyOriginWithoutCredentials(t *testing.T) {
	w, reached := corsRoundTrip(t, &config.RuntimeConfig{}, http.MethodGet, "https://evil.example")
	if !reached {
		t.Fatal("request did not reach the handler")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("a wildcard origin must never be paired with Allow-Credentials")
	}
}

func TestCorsWithATokenVariesOnOriginAndLeavesCrossOriginToAuth(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret"}

	w, _ := corsRoundTrip(t, cfg, http.MethodGet, "http://pinchtab:9867")
	if w.Header().Get("Vary") != "Origin" {
		t.Fatalf("Vary = %q, want Origin so a shared cache never serves one origin's allow header to another", w.Header().Get("Vary"))
	}

	w, reached := corsRoundTrip(t, cfg, http.MethodGet, "https://evil.example")
	if !reached {
		t.Fatal("a cross-origin non-preflight request must still reach auth, which decides")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("cross-origin with a token must get no allow headers, got origin=%q credentials=%q", w.Header().Get("Access-Control-Allow-Origin"), w.Header().Get("Access-Control-Allow-Credentials"))
	}
}

func TestCorsPreflightWithoutAnOriginIsAcceptedWithoutReachingTheHandler(t *testing.T) {
	w, reached := corsRoundTrip(t, &config.RuntimeConfig{Token: "secret"}, http.MethodOptions, "")
	if reached || w.Code != http.StatusNoContent {
		t.Fatalf("reached=%v code=%d, want 204 and no handler call", reached, w.Code)
	}
}
