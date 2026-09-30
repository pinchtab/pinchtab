package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/config"
)

// TestHandleNetworkExportPrefersRetainedBodyInOrder verifies the non-streaming
// export uses the buffer's retained body (no live CDP fetch — which would yield an
// empty body in this harness) and that the order-preserving incremental pipeline
// emits entries in capture order.
func TestHandleNetworkExportPrefersRetainedBodyInOrder(t *testing.T) {
	nm := observe.NewNetworkMonitor(100)
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

// failingExportEncoder fails at one named stage so every non-success exit of
// writeExportFile can be driven. The reservation the auto-named path creates is a
// real file, so each of these stages is a chance to abandon it.
type failingExportEncoder struct {
	failAt string
}

func (e failingExportEncoder) ContentType() string   { return "application/json" }
func (e failingExportEncoder) FileExtension() string { return ".har" }

func (e failingExportEncoder) Start(w io.Writer) error {
	if e.failAt == "start" {
		return fmt.Errorf("start refused")
	}
	_, err := w.Write([]byte("{"))
	return err
}

func (e failingExportEncoder) Encode(entry observe.ExportEntry) error {
	if e.failAt == "encode" {
		return fmt.Errorf("encode refused")
	}
	return nil
}

func (e failingExportEncoder) Finish() error {
	if e.failAt == "finish" {
		return fmt.Errorf("finish refused")
	}
	return nil
}

func exportDirEntries(t *testing.T, stateDir string) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(stateDir, "exports"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestAFailedAutoNamedExportLeavesNothingBehind(t *testing.T) {
	for _, failAt := range []string{"start", "encode", "finish"} {
		t.Run(failAt, func(t *testing.T) {
			stateDir := t.TempDir()
			h := New(&networkMockBridge{nm: observe.NewNetworkMonitor(10)}, &config.RuntimeConfig{StateDir: stateDir}, nil, nil, nil)

			req := httptest.NewRequest("GET", "/network/export", nil)
			err := h.writeExportFile(httptest.NewRecorder(), req, failingExportEncoder{failAt: failAt}, "har",
				func(emit func(observe.ExportEntry) error) error {
					return emit(observe.ExportEntry{})
				})
			if err == nil {
				t.Fatalf("precondition: an encoder failing at %s must fail the export", failAt)
			}

			for _, name := range exportDirEntries(t, stateDir) {
				t.Errorf("an export that failed at %s left %s behind; the reserved name now holds an empty export", failAt, name)
			}
		})
	}
}

// TestAFailedExportDoesNotPoisonTheNextName is the assertion that catches the
// consequence rather than the litter: an abandoned reservation pushes the NEXT
// export in the same second onto a -1 suffix, so the obvious name holds 0 bytes
// while the real export hides behind a suffix nobody asked for. Counting the files
// is what distinguishes that from a leak that merely litters.
func TestAFailedExportDoesNotPoisonTheNextName(t *testing.T) {
	stateDir := t.TempDir()
	h := New(&networkMockBridge{nm: observe.NewNetworkMonitor(10)}, &config.RuntimeConfig{StateDir: stateDir}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/network/export", nil)

	if err := h.writeExportFile(httptest.NewRecorder(), req, failingExportEncoder{failAt: "encode"}, "har",
		func(emit func(observe.ExportEntry) error) error {
			return emit(observe.ExportEntry{})
		}); err == nil {
		t.Fatal("precondition: the first export must fail")
	}

	if err := h.writeExportFile(httptest.NewRecorder(), req, failingExportEncoder{}, "har",
		func(emit func(observe.ExportEntry) error) error {
			return emit(observe.ExportEntry{})
		}); err != nil {
		t.Fatalf("second export: %v", err)
	}

	names := exportDirEntries(t, stateDir)
	if len(names) != 1 {
		t.Fatalf("exports dir holds %v, want exactly one file: a failed export must not leave a name behind for the next one to trip over", names)
	}
	if strings.Contains(names[0], "-1.") {
		t.Errorf("the successful export landed on %s; the suffix means a stale reservation still holds the unsuffixed name", names[0])
	}
	info, err := os.Stat(filepath.Join(stateDir, "exports", names[0]))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Errorf("%s is empty; the file under the export's own name is a reservation, not an export", names[0])
	}
}
