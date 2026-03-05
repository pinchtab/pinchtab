package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// fakeBridge creates a test server that mimics a bridge instance.
// It serves /tabs (for Locator discovery) and /tabs/{id}/* (for proxied requests).
func fakeBridge(t *testing.T, tabs []bridge.InstanceTab) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Locator queries /tabs for discovery.
		if r.URL.Path == "/tabs" {
			_ = json.NewEncoder(w).Encode(tabs)
			return
		}
		// Legacy discovery via /screencast/tabs returns raw CDP-like IDs.
		if r.URL.Path == "/screencast/tabs" {
			type remTab struct {
				ID string `json:"id"`
			}
			var raw []remTab
			for _, tab := range tabs {
				raw = append(raw, remTab{ID: tab.ID})
			}
			_ = json.NewEncoder(w).Encode(raw)
			return
		}
		// Proxied tab requests echo back the path so tests can verify routing.
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"proxied":true,"path":%q}`, r.URL.Path)
	}))
}

// setupOrchestratorWithInstance creates an orchestrator with a running instance
// backed by a fake bridge server. Returns the orchestrator and a cleanup func.
func setupOrchestratorWithInstance(t *testing.T, port string, tabs []bridge.InstanceTab) *Orchestrator {
	t.Helper()
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	// Inject a running instance directly.
	// Note: cmd is nil so instanceIsActive falls back to status check.
	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:     "inst_test1",
			Port:   port,
			Status: "running",
		},
		URL: "http://localhost:" + port,
	}
	o.mu.Lock()
	o.instances["inst_test1"] = inst
	o.mu.Unlock()

	// Sync to Manager so Locator can discover it.
	o.syncInstanceToManager(&inst.Instance)

	return o
}

func TestProxyTabRequest_ManagerCacheHit(t *testing.T) {
	tabs := []bridge.InstanceTab{{ID: "tab_abc123", InstanceID: "inst_test1"}}
	srv := fakeBridge(t, tabs)
	defer srv.Close()

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	o := setupOrchestratorWithInstance(t, port, tabs)

	// Pre-populate the Locator cache (simulates a previous lookup).
	o.instanceMgr.Locator.Register("tab_abc123", "inst_test1")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /tabs/{id}/snapshot", o.proxyTabRequest)

	req := httptest.NewRequest("GET", "/tabs/tab_abc123/snapshot", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["proxied"] != true {
		t.Error("request was not proxied to bridge")
	}
}

func TestProxyTabRequest_ManagerMiss_LegacyFallback(t *testing.T) {
	// Tab uses a CDP-like raw ID that the Locator won't find,
	// but the legacy lookup with idMgr translation would.
	// Here we test with a hash-based ID that IS discoverable by legacy.
	tabs := []bridge.InstanceTab{{ID: "tab_def456", InstanceID: "inst_test1"}}
	srv := fakeBridge(t, tabs)
	defer srv.Close()

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	o := setupOrchestratorWithInstance(t, port, tabs)

	// Do NOT populate cache — force Locator miss, then legacy fallback.
	// Override fetchTabs to use our fake server.
	o.client = srv.Client()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /tabs/{id}/snapshot", o.proxyTabRequest)

	req := httptest.NewRequest("GET", "/tabs/tab_def456/snapshot", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// The Locator will query /tabs on the bridge, find tab_def456, and route.
	if rec.Code != 200 {
		t.Fatalf("expected 200 via fallback, got %d: %s", rec.Code, rec.Body.String())
	}

	// After fallback, tab should now be cached in Locator.
	if o.instanceMgr.Locator.CacheSize() == 0 {
		t.Error("expected Locator cache to be populated after discovery")
	}
}

func TestProxyTabRequest_TabNotFound(t *testing.T) {
	tabs := []bridge.InstanceTab{{ID: "tab_exists", InstanceID: "inst_test1"}}
	srv := fakeBridge(t, tabs)
	defer srv.Close()

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	o := setupOrchestratorWithInstance(t, port, tabs)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /tabs/{id}/snapshot", o.proxyTabRequest)

	req := httptest.NewRequest("GET", "/tabs/tab_nonexistent/snapshot", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for unknown tab, got %d", rec.Code)
	}
}

func TestProxyTabRequest_MissingTabID(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	// Call proxyTabRequest directly with no path value set.
	req := httptest.NewRequest("GET", "/tabs/snapshot", nil)
	rec := httptest.NewRecorder()
	o.proxyTabRequest(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for missing tab ID, got %d", rec.Code)
	}
}

func TestSyncInstanceToManager(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst := &bridge.Instance{
		ID:     "inst_sync1",
		Port:   "9868",
		Status: "running",
	}

	o.syncInstanceToManager(inst)

	// Verify it's in the Manager's repository.
	got, ok := o.instanceMgr.Repo.Get("inst_sync1")
	if !ok {
		t.Fatal("instance not found in Manager repo after sync")
	}
	if got.Port != "9868" {
		t.Errorf("port = %s, want 9868", got.Port)
	}
}

func TestSetAllocationPolicy(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	// Default is FCFS.
	if err := o.SetAllocationPolicy("round_robin"); err != nil {
		t.Fatalf("SetAllocationPolicy(round_robin) failed: %v", err)
	}

	if err := o.SetAllocationPolicy("random"); err != nil {
		t.Fatalf("SetAllocationPolicy(random) failed: %v", err)
	}

	if err := o.SetAllocationPolicy("fcfs"); err != nil {
		t.Fatalf("SetAllocationPolicy(fcfs) failed: %v", err)
	}

	if err := o.SetAllocationPolicy("nonexistent"); err == nil {
		t.Error("expected error for unknown policy")
	}
}

func TestInstanceManager_Getter(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	mgr := o.InstanceManager()
	if mgr == nil {
		t.Fatal("InstanceManager() returned nil")
	}
	if mgr.Repo == nil || mgr.Locator == nil || mgr.Allocator == nil {
		t.Error("Manager components should not be nil")
	}
}

// mockCmd is defined in process_test.go
