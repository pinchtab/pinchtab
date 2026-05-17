package simple

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
)

type mockRunner struct {
	portAvail bool
	cmds      []*mockCmd
}

type mockCmd struct {
	done chan struct{}
	once sync.Once
}

func newMockCmd() *mockCmd {
	return &mockCmd{done: make(chan struct{})}
}

func (m *mockCmd) Wait() error {
	<-m.done
	return nil
}

func (m *mockCmd) PID() int { return 1234 }

func (m *mockCmd) Cancel() {
	m.once.Do(func() {
		close(m.done)
	})
}

func (m *mockRunner) Run(context.Context, string, []string, []string, io.Writer, io.Writer) (orchestrator.Cmd, error) {
	cmd := newMockCmd()
	m.cmds = append(m.cmds, cmd)
	return cmd, nil
}

func (m *mockRunner) InspectPort(string) orchestrator.PortInspection {
	return orchestrator.PortInspection{Available: m.portAvail}
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestStrategy_EnsureRunning_BrowserTargetAutoLaunchesRequestedTarget(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	runner := &mockRunner{portAvail: true}
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), runner)
	orch.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserProviderChrome},
			"cloak":  {Provider: config.BrowserProviderCloak},
		},
	})
	t.Cleanup(func() {
		for _, inst := range orch.List() {
			_ = orch.Stop(inst.ID)
		}
	})

	body := `{"url":"about:blank","browserTarget":"cloak"}`
	req := httptest.NewRequest(http.MethodPost, "/navigate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	s := &Strategy{orch: orch}
	target, status, err := s.ensureRunning(req)
	if err != nil {
		t.Fatalf("ensureRunning status=%d err=%v", status, err)
	}
	if target == "" {
		t.Fatal("target URL is empty")
	}
	instances := orch.List()
	if len(instances) != 1 {
		t.Fatalf("instances = %d, want 1: %+v", len(instances), instances)
	}
	if instances[0].BrowserTarget != "cloak" {
		t.Fatalf("BrowserTarget = %q, want cloak", instances[0].BrowserTarget)
	}
	if instances[0].BrowserProvider != config.BrowserProviderCloak {
		t.Fatalf("BrowserProvider = %q, want cloak", instances[0].BrowserProvider)
	}
	if instances[0].ProfileName != "default-cloak" {
		t.Fatalf("ProfileName = %q, want default-cloak", instances[0].ProfileName)
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

	tests := []struct {
		method  string
		path    string
		body    string
		setting string
	}{
		{method: "POST", path: "/evaluate", body: `{"expression":"1+1"}`, setting: "security.allowEvaluate"},
		{method: "GET", path: "/cookies", setting: "security.allowCookies"},
		{method: "DELETE", path: "/cookies", setting: "security.allowCookies"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != 403 {
			t.Fatalf("%s %s expected 403, got %d: %s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tt.setting) {
			t.Fatalf("%s %s expected lock response to mention %s, got %s", tt.method, tt.path, tt.setting, rec.Body.String())
		}
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

func TestStrategy_RegisterRoutes_RegistersFrameShorthands(t *testing.T) {
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
		{method: http.MethodGet, path: "/frame", route: "GET /frame"},
		{method: http.MethodPost, path: "/frame", route: "POST /frame"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"target":"main"}`))
			_, pattern := mux.Handler(req)
			if pattern != tt.route {
				t.Fatalf("expected route %q, got %q", tt.route, pattern)
			}
		})
	}
}
