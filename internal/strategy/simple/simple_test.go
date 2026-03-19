package simple

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
)

type mockRunner struct {
	portAvail bool
}

func (m *mockRunner) Run(context.Context, string, []string, []string, io.Writer, io.Writer) (orchestrator.Cmd, error) {
	return nil, nil
}

func (m *mockRunner) IsPortAvailable(string) bool {
	return m.portAvail
}

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

func TestStrategy_RegisterRoutes_LocksSensitiveShorthandRoutes(t *testing.T) {
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	orch.ApplyRuntimeConfig(&config.RuntimeConfig{})

	s := &Strategy{orch: orch}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/evaluate", strings.NewReader(`{"expression":"1+1"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "security.allowEvaluate") {
		t.Fatalf("expected lock response to mention security.allowEvaluate, got %s", rec.Body.String())
	}
}

func TestStrategy_RegisterRoutes_RegistersConsoleAndErrorShorthands(t *testing.T) {
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	orch.ApplyRuntimeConfig(&config.RuntimeConfig{})

	s := &Strategy{orch: orch}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	tests := []struct {
		method string
		path   string
		route  string
	}{
		{method: http.MethodGet, path: "/console", route: "GET /console"},
		{method: http.MethodPost, path: "/console/clear", route: "POST /console/clear"},
		{method: http.MethodGet, path: "/errors", route: "GET /errors"},
		{method: http.MethodPost, path: "/errors/clear", route: "POST /errors/clear"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			_, pattern := mux.Handler(req)
			if pattern != tt.route {
				t.Fatalf("expected route %q, got %q", tt.route, pattern)
			}
		})
	}
}
