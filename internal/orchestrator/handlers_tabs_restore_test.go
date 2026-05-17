package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/tabstate"
)

// fakeTabBackend returns an http.Handler that pretends to be the per-instance
// bridge's /tab endpoint. It accepts POST /tab with {action:new, url}, returns
// {tabId: "tab_<n>", url: "<url>"}, and tracks the URLs it received.
type fakeTabBackend struct {
	mu     sync.Mutex
	opens  []string
	count  atomic.Int64
	failFn func(url string) bool // optional: return true to fail this URL
}

func (f *fakeTabBackend) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tab" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Action string `json:"action"`
			URL    string `json:"url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if f.failFn != nil && f.failFn(body.URL) {
			http.Error(w, "synthetic failure", http.StatusBadGateway)
			return
		}

		f.mu.Lock()
		f.opens = append(f.opens, body.URL)
		f.mu.Unlock()
		n := f.count.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tabId":"tab_%d","url":%q,"title":""}`, n, body.URL)
	})
}

// writeTabState seeds a tabs.json snapshot into profileDir.
func writeTabState(t *testing.T, profileDir string, urls ...string) {
	t.Helper()
	tabs := make([]tabstate.PersistedTab, 0, len(urls))
	for i, u := range urls {
		tabs = append(tabs, tabstate.PersistedTab{
			ID:    fmt.Sprintf("old_%d", i),
			URL:   u,
			Title: "T",
		})
	}
	if err := tabstate.Persist(profileDir, tabstate.Snapshot{
		InstanceID: "inst_seeded",
		Tabs:       tabs,
	}); err != nil {
		t.Fatalf("seed tabstate: %v", err)
	}
}

// newRestoreFixture wires up an orchestrator with one running instance whose
// /tab requests land on `backend`. The instance's profileName is "smiles" and
// its profile dir is baseDir/smiles (which mirrors orchestrator.profilePathFor
// when ProfileManager is nil — the test setup keeps profiles unset for clarity).
func newRestoreFixture(t *testing.T, backend *httptest.Server) (*Orchestrator, string) {
	t.Helper()
	baseDir := t.TempDir()
	o := NewOrchestrator(baseDir)
	o.client = backend.Client()

	profileName := "smiles"
	profileDir := filepath.Join(baseDir, profileName)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}

	port := strings.TrimPrefix(backend.URL, "http://localhost:")
	port = strings.TrimPrefix(port, "http://127.0.0.1:")

	o.instances["inst_smiles"] = &InstanceInternal{
		Instance: bridge.Instance{
			ID:          "inst_smiles",
			ProfileName: profileName,
			Port:        port,
			Status:      "running",
		},
		URL: backend.URL,
		cmd: &mockCmd{pid: 1, isAlive: true},
	}

	orig := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	t.Cleanup(func() { processAliveFunc = orig })

	return o, profileDir
}

func doRestore(t *testing.T, o *Orchestrator, instanceID string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/instances/"+instanceID+"/tabs/restore", bytes.NewReader(nil))
	req.SetPathValue("id", instanceID)
	w := httptest.NewRecorder()
	o.handleInstanceTabsRestore(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("parse body %q: %v", body, err)
		}
		return w, parsed
	}
	// Non-200 — surface the body as the test failure message via map.
	return w, map[string]any{"__error_body": string(body)}
}

func TestRestoreTabs_ReopensAllURLs(t *testing.T) {
	backend := &fakeTabBackend{}
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	o, profileDir := newRestoreFixture(t, srv)
	writeTabState(t, profileDir,
		"https://secure.sarsefiling.co.za/TaxRefund/SOA",
		"https://enterprisests.standardbank.co.za/login",
	)

	w, body := doRestore(t, o, "inst_smiles")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %v", w.Code, body)
	}
	if got, want := body["ok"], true; got != want {
		t.Errorf("ok = %v, want %v", got, want)
	}
	restored, _ := body["restored"].([]any)
	if len(restored) != 2 {
		t.Fatalf("restored len = %d, want 2 — body: %v", len(restored), body)
	}
	skipped, _ := body["skipped"].([]any)
	if len(skipped) != 0 {
		t.Errorf("skipped len = %d, want 0 — body: %v", len(skipped), body)
	}

	// New tab IDs were synthesized by the fake backend.
	for i, entry := range restored {
		m := entry.(map[string]any)
		newID, _ := m["newTabId"].(string)
		if !strings.HasPrefix(newID, "tab_") {
			t.Errorf("restored[%d].newTabId = %q, want tab_*", i, newID)
		}
		url, _ := m["url"].(string)
		if url == "" {
			t.Errorf("restored[%d].url is empty", i)
		}
	}

	// The fake backend should have seen the same URLs we persisted, in order.
	backend.mu.Lock()
	gotURLs := append([]string(nil), backend.opens...)
	backend.mu.Unlock()
	if len(gotURLs) != 2 {
		t.Fatalf("backend saw %d open requests, want 2", len(gotURLs))
	}
}

func TestRestoreTabs_MissingState_ReturnsEmpty(t *testing.T) {
	backend := &fakeTabBackend{}
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	o, _ := newRestoreFixture(t, srv)
	// Don't write any tabstate file.

	w, body := doRestore(t, o, "inst_smiles")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %v", w.Code, body)
	}
	restored, _ := body["restored"].([]any)
	if len(restored) != 0 {
		t.Errorf("restored len = %d, want 0", len(restored))
	}
	skipped, _ := body["skipped"].([]any)
	if len(skipped) != 0 {
		t.Errorf("skipped len = %d, want 0", len(skipped))
	}
}

func TestRestoreTabs_InstanceNotFound(t *testing.T) {
	backend := &fakeTabBackend{}
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	o, _ := newRestoreFixture(t, srv)
	w, _ := doRestore(t, o, "inst_nope")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestRestoreTabs_InstanceNotRunning(t *testing.T) {
	backend := &fakeTabBackend{}
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	o, _ := newRestoreFixture(t, srv)
	o.instances["inst_smiles"].Status = "stopped"
	w, _ := doRestore(t, o, "inst_smiles")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestRestoreTabs_PartialFailure_StillReturns200(t *testing.T) {
	backend := &fakeTabBackend{
		failFn: func(u string) bool { return strings.Contains(u, "fails") },
	}
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	o, profileDir := newRestoreFixture(t, srv)
	writeTabState(t, profileDir,
		"https://ok.example/",
		"https://fails.example/",
		"https://ok-two.example/",
	)

	w, body := doRestore(t, o, "inst_smiles")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %v", w.Code, body)
	}
	restored, _ := body["restored"].([]any)
	skipped, _ := body["skipped"].([]any)
	if len(restored) != 2 {
		t.Errorf("restored len = %d, want 2", len(restored))
	}
	if len(skipped) != 1 {
		t.Errorf("skipped len = %d, want 1", len(skipped))
	} else {
		entry := skipped[0].(map[string]any)
		if u, _ := entry["url"].(string); !strings.Contains(u, "fails") {
			t.Errorf("skipped url = %q, want one containing 'fails'", u)
		}
		if r, _ := entry["reason"].(string); r == "" {
			t.Errorf("skipped reason is empty")
		}
	}
}

func TestRestoreTabs_PreservesSavedAt(t *testing.T) {
	backend := &fakeTabBackend{}
	srv := httptest.NewServer(backend.handler())
	defer srv.Close()

	o, profileDir := newRestoreFixture(t, srv)
	writeTabState(t, profileDir, "https://example/")

	_, body := doRestore(t, o, "inst_smiles")
	restored, _ := body["restored"].([]any)
	if len(restored) != 1 {
		t.Fatalf("restored len = %d, want 1", len(restored))
	}
	entry := restored[0].(map[string]any)
	if _, ok := entry["fromSavedAt"]; !ok {
		t.Errorf("restored entry missing fromSavedAt")
	}
}

func TestProfilePathFor_FallsBackToBaseDirJoin(t *testing.T) {
	o := NewOrchestrator("/var/profiles")
	got := o.profilePathFor("smiles")
	if want := filepath.Join("/var/profiles", "smiles"); got != want {
		t.Errorf("profilePathFor = %q, want %q", got, want)
	}
	if got := o.profilePathFor(""); got != "" {
		t.Errorf("profilePathFor(\"\") = %q, want \"\"", got)
	}
}
