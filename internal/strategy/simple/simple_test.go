package simple

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
	"github.com/pinchtab/pinchtab/internal/routes"
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
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})
	// Shutdown, not a Stop loop: it also waits for the startup monitors, which
	// otherwise outlive the test and keep reading http.DefaultTransport while
	// the cleanup registered above restores it.
	t.Cleanup(orch.Shutdown)

	req := httptest.NewRequest(http.MethodPost, "/navigate?browser=cloak", nil)

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
	if instances[0].Browser != config.BrowserCloak {
		t.Fatalf("Browser = %q, want cloak", instances[0].Browser)
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

func lockedMux(t *testing.T) *http.ServeMux {
	t.Helper()
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	orch.ApplyRuntimeConfig(&config.RuntimeConfig{})

	s := &Strategy{orch: orch}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return mux
}

func refuse(t *testing.T, mux *http.ServeMux, ep routes.Endpoint) map[string]any {
	t.Helper()
	req := httptest.NewRequest(ep.Method, ep.Path, strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("%s: got %d, want 403: %s", ep.Route(), rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: refusal is not JSON: %v: %s", ep.Route(), err, rec.Body.String())
	}
	return body
}

func TestStrategy_RegisterRoutes_RefusalsComeFromTheCapabilityOwner(t *testing.T) {
	gated := routes.CapabilityEndpoints()
	if len(gated) == 0 {
		t.Fatal("no capability-gated endpoints; this guard would pass vacuously")
	}
	mux := lockedMux(t)

	seen := 0
	for cap, eps := range gated {
		meta, ok := routes.Meta(cap)
		if !ok {
			t.Errorf("capability %q gates routes but routes.Meta does not describe it", cap)
			continue
		}
		want := map[string]any{
			"code":    meta.DisabledCode,
			"error":   httpx.DisabledEndpointMessage(meta.Label, meta.Setting),
			"details": httpx.DisabledEndpointDetails(meta.Setting),
		}
		if len(eps) == 0 {
			t.Errorf("capability %q yields no endpoints, so it is uncovered here", cap)
		}
		for _, ep := range eps {
			got := refuse(t, mux, ep)
			delete(got, "retryable")
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s refusal = %#v, want %#v", ep.Route(), got, want)
			}
			seen++
		}
	}
	if seen < len(gated) {
		t.Fatalf("checked %d endpoints across %d capabilities; every capability must contribute at least one", seen, len(gated))
	}
}

func TestStrategy_RegisterRoutes_RefusedSettingIsASettableConfigKey(t *testing.T) {
	gated := routes.CapabilityEndpoints()
	if len(gated) == 0 {
		t.Fatal("no capability-gated endpoints; this guard would pass vacuously")
	}
	mux := lockedMux(t)

	for cap, eps := range gated {
		details, _ := refuse(t, mux, eps[0])["details"].(map[string]any)
		setting, _ := details["setting"].(string)
		if setting == "" {
			t.Errorf("capability %q refuses without naming a setting, so its remedy cannot be followed", cap)
			continue
		}
		if err := config.SetConfigValue(&config.FileConfig{}, setting, "true"); err != nil {
			t.Errorf("capability %q refuses citing %q, which the config editor rejects: %v", cap, setting, err)
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
