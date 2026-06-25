package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// TestHandleNetworkExportPrefersRetainedBodyInOrder verifies the non-streaming
// export uses the buffer's retained body (no live CDP fetch — which would yield an
// empty body in this harness) and that the order-preserving incremental pipeline
// emits entries in capture order.
func TestHandleNetworkExportPrefersRetainedBodyInOrder(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	wantBodies := []string{"body-A", "body-B", "body-C"}
	for i, body := range wantBodies {
		buf.Add(bridge.NetworkEntry{
			RequestID:    fmt.Sprintf("r%d", i),
			URL:          fmt.Sprintf("https://api.example.com/%d", i),
			Method:       "GET",
			Status:       200,
			ResourceType: "XHR",
			Finished:     true,
			ResponseBody: body,
			BodyRetained: true,
		})
	}
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/export?format=ndjson&body=true", nil)
	w := httptest.NewRecorder()
	h.HandleNetworkExport(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != len(wantBodies) {
		t.Fatalf("expected %d ndjson lines, got %d: %q", len(wantBodies), len(lines), w.Body.String())
	}
	for i, line := range lines {
		var e struct {
			Request struct {
				URL string `json:"url"`
			} `json:"request"`
			Response struct {
				Content struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d decode: %v (%q)", i, err, line)
		}
		if wantURL := fmt.Sprintf("https://api.example.com/%d", i); e.Request.URL != wantURL {
			t.Errorf("line %d: capture order broken, url=%s want %s", i, e.Request.URL, wantURL)
		}
		if e.Response.Content.Text != wantBodies[i] {
			t.Errorf("line %d: body=%q want %q (retained body not used?)", i, e.Response.Content.Text, wantBodies[i])
		}
	}
}

func TestCleanupStaleTmpExports(t *testing.T) {
	stateDir := t.TempDir()
	exportDir := filepath.Join(stateDir, "exports")
	if err := os.MkdirAll(exportDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create a stale .tmp file (backdate mtime well past the 5-min threshold).
	stalePath := filepath.Join(exportDir, "network-old.har.tmp")
	if err := os.WriteFile(stalePath, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(stalePath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	// Create a fresh .tmp file that should be kept (could be in-flight).
	freshPath := filepath.Join(exportDir, "network-new.ndjson.tmp")
	if err := os.WriteFile(freshPath, []byte("fresh"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create a regular completed file that should never be touched.
	completedPath := filepath.Join(exportDir, "session.har")
	if err := os.WriteFile(completedPath, []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}

	CleanupStaleTmpExports(stateDir)

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Error("stale .tmp file should have been removed")
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Error("fresh .tmp file should have been kept")
	}
	if _, err := os.Stat(completedPath); err != nil {
		t.Error("completed .har file should have been kept")
	}
}

func TestCleanupStaleTmpExports_NoDir(t *testing.T) {
	// Should not panic when exports/ doesn't exist.
	CleanupStaleTmpExports(t.TempDir())
}
