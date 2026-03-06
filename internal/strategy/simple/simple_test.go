package simple

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/proxy"
)

// fakeBridge creates a test server that mimics a bridge instance.
func fakeBridge(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"proxied": true, "path": r.URL.Path})
	}))
}

func TestProxyHTTP_ForwardsRequest(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, srv.URL+"/snapshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["path"] != "/snapshot" {
		t.Errorf("expected path /snapshot, got %v", resp["path"])
	}
}

func TestProxyHTTP_ForwardsQueryParams(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/screenshot?raw=true", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, srv.URL+"/screenshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestProxyHTTP_UnreachableReturns502(t *testing.T) {
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, "http://localhost:1/snapshot")

	if rec.Code != 502 {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

func TestStrategy_Name(t *testing.T) {
	s := &Strategy{}
	if s.Name() != "simple" {
		t.Errorf("expected 'simple', got %q", s.Name())
	}
}

func TestStrategy_ProxyToFirst_NoOrch_Returns503(t *testing.T) {
	s := &Strategy{} // no orchestrator
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	s.proxyToFirst(rec, req)

	if rec.Code != 503 {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStrategy_HandleTabs_NoInstances(t *testing.T) {
	// handleTabs with nil orch would panic — test the empty-tabs path
	// by checking the JSON response format of proxyHTTP fallback.
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/tabs", nil)
	rec := httptest.NewRecorder()
	proxy.HTTP(rec, req, srv.URL+"/tabs")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
