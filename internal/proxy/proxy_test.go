package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeBridge(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proxied": true,
			"path":    r.URL.Path,
			"query":   r.URL.RawQuery,
		})
	}))
}

func TestHTTP_ForwardsRequest(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/snapshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["path"] != "/snapshot" {
		t.Errorf("expected path /snapshot, got %v", resp["path"])
	}
}

func TestHTTP_ForwardsQueryParams(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/screenshot?raw=true", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/screenshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["query"] != "raw=true" {
		t.Errorf("expected query raw=true, got %v", resp["query"])
	}
}

func TestHTTP_UnreachableReturns502(t *testing.T) {
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, "http://localhost:1/snapshot")

	if rec.Code != 502 {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

func TestHTTP_CopiesResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(201)
	}))
	defer srv.Close()

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/test")

	if rec.Code != 201 {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom") != "test-value" {
		t.Errorf("expected X-Custom header, got %q", rec.Header().Get("X-Custom"))
	}
}

func TestHTTP_UsesSharedClient(t *testing.T) {
	if DefaultClient == nil {
		t.Fatal("DefaultClient should not be nil")
	}
	if DefaultClient.Timeout != 60*1e9 { // 60 seconds in nanoseconds
		t.Errorf("expected 60s timeout, got %s", DefaultClient.Timeout)
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   bool
	}{
		{"no upgrade", http.Header{}, false},
		{"websocket", http.Header{"Upgrade": {"websocket"}}, true},
		{"WebSocket", http.Header{"Upgrade": {"WebSocket"}}, true},
		{"other", http.Header{"Upgrade": {"h2c"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header = tt.header
			if got := isWebSocketUpgrade(r); got != tt.want {
				t.Errorf("isWebSocketUpgrade() = %v, want %v", got, tt.want)
			}
		})
	}
}
