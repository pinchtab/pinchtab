package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/heapsnap"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func memoryHandlers(t *testing.T, allow bool) (*Handlers, string) {
	t.Helper()
	stateDir := t.TempDir()
	cfg := &config.RuntimeConfig{StateDir: stateDir, AllowMemory: allow, ActionTimeout: time.Second}
	return New(&mockBridge{}, cfg, nil, nil, nil), stateDir
}

func seedSnapshot(t *testing.T, stateDir, id, body string) {
	t.Helper()
	dir := heapsnap.Dir(stateDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+heapsnap.Ext), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func committedFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "heapsnap", "testdata", "small.heapsnapshot"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func summaryRequest(h *Handlers, id, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/memory/snapshot/"+id+"/summary"+query, nil)
	req.SetPathValue("snapshotId", id)
	w := httptest.NewRecorder()
	h.HandleMemorySnapshotSummary(w, req)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Code
}

func TestMemoryCapabilityGatesSnapshotAndSummaryButNotUsage(t *testing.T) {
	h, stateDir := memoryHandlers(t, false)
	seedSnapshot(t, stateDir, "heap_seeded", committedFixture(t))

	w := httptest.NewRecorder()
	h.HandleMemorySnapshot(w, httptest.NewRequest(http.MethodPost, "/memory/snapshot", nil))
	if w.Code != http.StatusForbidden || errorCode(t, w) != "memory_disabled" {
		t.Fatalf("snapshot with allowMemory off = %d %s, want 403 memory_disabled", w.Code, w.Body.String())
	}

	w = summaryRequest(h, "heap_seeded", "")
	if w.Code != http.StatusForbidden || errorCode(t, w) != "memory_disabled" {
		t.Fatalf("summary with allowMemory off = %d %s, want 403 memory_disabled", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.HandleMemory(w, httptest.NewRequest(http.MethodGet, "/memory", nil))
	if errorCode(t, w) == "memory_disabled" {
		t.Fatalf("GET /memory refused with the capability code: %s", w.Body.String())
	}
}

func TestMemoryUsageRefusesAMalformedGC(t *testing.T) {
	h, _ := memoryHandlers(t, false)
	w := httptest.NewRecorder()
	h.HandleMemory(w, httptest.NewRequest(http.MethodGet, "/memory?gc=maybe", nil))
	if w.Code != http.StatusBadRequest || errorCode(t, w) != "bad_gc" {
		t.Fatalf("gc=maybe = %d %s, want 400 bad_gc", w.Code, w.Body.String())
	}
}

func TestMemorySummaryReadsASavedSnapshot(t *testing.T) {
	h, stateDir := memoryHandlers(t, true)
	seedSnapshot(t, stateDir, "heap_seeded", committedFixture(t))

	w := summaryRequest(h, "heap_seeded", "?top=2")
	if w.Code != http.StatusOK {
		t.Fatalf("summary = %d %s", w.Code, w.Body.String())
	}
	var got memorySummaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "heap_seeded" || got.Top != 2 || got.NodeCount != 20 || got.EdgeCount != 6 {
		t.Fatalf("summary header = %+v", got)
	}
	if len(got.TopBySize) != 2 || got.TopBySize[0].Name != "Array" || len(got.DuplicateStrings) != 2 || got.DuplicateStrings[0].Value != "secret-token-abc" {
		t.Fatalf("summary tables = %+v / %+v", got.TopBySize, got.DuplicateStrings)
	}
}

func TestMemorySummaryRefusals(t *testing.T) {
	h, stateDir := memoryHandlers(t, true)
	seedSnapshot(t, stateDir, "heap_broken", strings.Replace(committedFixture(t), `"node_count":20`, `"node_count":3`, 1))

	cases := []struct {
		id, query string
		status    int
		code      string
	}{
		{"..", "", http.StatusBadRequest, "bad_snapshot_id"},
		{"heap_missing", "", http.StatusNotFound, "memory_snapshot_not_found"},
		{"heap_broken", "?top=0", http.StatusBadRequest, "bad_top"},
		{"heap_broken", "", http.StatusUnprocessableEntity, "memory_snapshot_invalid"},
	}
	for _, tc := range cases {
		w := summaryRequest(h, tc.id, tc.query)
		if w.Code != tc.status || errorCode(t, w) != tc.code {
			t.Errorf("%s%s = %d %s, want %d %s", tc.id, tc.query, w.Code, w.Body.String(), tc.status, tc.code)
		}
	}
	w := summaryRequest(h, "heap_broken", "")
	if !strings.Contains(w.Body.String(), `"section":"nodes"`) {
		t.Errorf("malformed snapshot refusal does not name the section: %s", w.Body.String())
	}
}

func TestAFailedMemorySnapshotLeavesNoFile(t *testing.T) {
	h, stateDir := memoryHandlers(t, true)
	w := httptest.NewRecorder()
	h.HandleMemorySnapshot(w, httptest.NewRequest(http.MethodPost, "/memory/snapshot", strings.NewReader(`{"tabId":"tab1"}`)))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("snapshot on a tab with no CDP target = %d %s, want 500", w.Code, w.Body.String())
	}
	entries, err := os.ReadDir(heapsnap.Dir(stateDir))
	if err != nil {
		t.Fatalf("the snapshot never reserved a path, so this proves nothing: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("a failed snapshot left %d file(s) behind", len(entries))
	}
}

func browserMemoryHandlers(t *testing.T, cfg *config.RuntimeConfig) *Handlers {
	t.Helper()
	ctx := testbrowser.Tab(t)
	page := `<html><body><script>window.keep = Array.from({length: 40}, () => [1, 2, 3]);</script></body></html>`
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html,"+page)); err != nil {
		t.Fatal(err)
	}
	cfg.ActionTimeout = 10 * time.Second
	cfg.DefaultBrowser = config.BrowserChrome
	cfg.StateDir = t.TempDir()
	b := browserBridge(ctx, cfg)
	b.RegisterTab("tab-mem", ctx)
	return New(b, cfg, nil, nil, nil)
}

func TestMemorySnapshotWritesAFileTheSummaryReads(t *testing.T) {
	h := browserMemoryHandlers(t, &config.RuntimeConfig{AllowMemory: true})

	w := httptest.NewRecorder()
	h.HandleMemory(w, httptest.NewRequest(http.MethodGet, "/memory?tabId=tab-mem&gc=true", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("usage = %d %s", w.Code, w.Body.String())
	}
	var usage map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"usedJSHeapSize", "totalJSHeapSize", "jsHeapSizeLimit", "documents", "nodes", "listeners", "frames"} {
		if _, ok := usage[key]; !ok {
			t.Errorf("usage lacks %s: %s", key, w.Body.String())
		}
	}

	w = httptest.NewRecorder()
	h.HandleMemorySnapshot(w, httptest.NewRequest(http.MethodPost, "/memory/snapshot", strings.NewReader(`{"tabId":"tab-mem"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot = %d %s", w.Code, w.Body.String())
	}
	var snap memorySnapshotResponse
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(snap.Path)
	if err != nil || info.Size() != snap.Bytes || snap.NodeCount == 0 || !strings.HasPrefix(snap.ID, heapsnap.IDPrefix) {
		t.Fatalf("snapshot response %+v vs file %v (%v)", snap, info, err)
	}
	if filepath.Dir(snap.Path) != heapsnap.Dir(h.Config.StateDir) {
		t.Fatalf("snapshot landed in %s, want the server-controlled %s", filepath.Dir(snap.Path), heapsnap.Dir(h.Config.StateDir))
	}

	w = summaryRequest(h, snap.ID, "?top=50")
	if w.Code != http.StatusOK {
		t.Fatalf("summary = %d %s", w.Code, w.Body.String())
	}
	var summary memorySummaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.NodeCount != snap.NodeCount {
		t.Fatalf("summary counted %d nodes, snapshot reported %d", summary.NodeCount, snap.NodeCount)
	}
	found := false
	for _, c := range summary.TopByCount {
		found = found || c.Name == "Array"
	}
	if !found {
		t.Fatalf("Array missing from the top constructors: %+v", summary.TopByCount)
	}
}

func TestMemorySnapshotOverTheCapLeavesNoFile(t *testing.T) {
	h := browserMemoryHandlers(t, &config.RuntimeConfig{AllowMemory: true, MemorySnapshotMaxBytes: 4096})

	w := httptest.NewRecorder()
	h.HandleMemorySnapshot(w, httptest.NewRequest(http.MethodPost, "/memory/snapshot", strings.NewReader(`{"tabId":"tab-mem"}`)))
	if w.Code != http.StatusRequestEntityTooLarge || errorCode(t, w) != memorySnapshotTooLargeCode {
		t.Fatalf("capped snapshot = %d %s, want 413 %s", w.Code, w.Body.String(), memorySnapshotTooLargeCode)
	}
	entries, err := os.ReadDir(heapsnap.Dir(h.Config.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("an aborted snapshot left %d file(s) behind", len(entries))
	}
}
