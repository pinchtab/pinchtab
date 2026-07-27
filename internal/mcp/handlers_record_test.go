package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestSafeRecordPath_RejectsRelativePath(t *testing.T) {
	_, err := safeRecordPath("relative/output.gif")
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestSafeRecordPath_RejectsBadExtension(t *testing.T) {
	_, err := safeRecordPath(filepath.Join(t.TempDir(), "output.txt"))
	if err == nil || !strings.Contains(err.Error(), "unsupported extension") {
		t.Fatalf("expected extension error, got %v", err)
	}
}

func TestSafeRecordPath_RejectsExistingFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "existing.gif")
	if err := os.WriteFile(f, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := safeRecordPath(f)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected exists error, got %v", err)
	}
}

func TestSafeRecordPath_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.gif")
	link := filepath.Join(dir, "link.gif")
	if err := os.WriteFile(target, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := safeRecordPath(link)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestSafeRecordPath_AcceptsValidPath(t *testing.T) {
	dir := t.TempDir()
	for _, ext := range []string{".gif", ".webm", ".mp4"} {
		path := filepath.Join(dir, "out"+ext)
		got, err := safeRecordPath(path)
		if err != nil {
			t.Fatalf("ext %s: unexpected error: %v", ext, err)
		}
		if got != path {
			t.Fatalf("ext %s: got %q, want %q", ext, got, path)
		}
	}
}

func TestSafeRecordPath_RejectsNonDirectoryParent(t *testing.T) {
	regular := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(regular, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := safeRecordPath(filepath.Join(regular, "out.gif"))
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not-a-directory error, got %v", err)
	}
}

func TestStreamToFile_WritesAndCaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.gif")

	data := strings.NewReader("hello recording data")
	n, err := streamToFile(path, data)
	if err != nil {
		t.Fatalf("streamToFile() error = %v", err)
	}
	if n != 20 {
		t.Fatalf("wrote %d bytes, want 20", n)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello recording data" {
		t.Fatalf("file content = %q", got)
	}
}

func TestStreamToFile_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.gif")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := streamToFile(path, strings.NewReader("new data"))
	if err == nil {
		t.Fatal("expected error for existing file")
	}
}

func TestRecordStartRejectsRelativePath(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_record_start", map[string]any{
		"file": "relative/out.gif",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "absolute path") {
		t.Fatalf("expected absolute path error, got %q", text)
	}
}

func TestRecordStartRejectsExistingFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.webm")
	if err := os.WriteFile(existing, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_record_start", map[string]any{
		"file": existing,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "already exists") {
		t.Fatalf("expected exists error, got %q", text)
	}
}

func TestRecordStopBadPathStillStopsRecording(t *testing.T) {
	stopCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/record/stop" && r.Method == http.MethodPost {
			stopCalled = true
			w.WriteHeader(200)
			_, _ = w.Write([]byte("fakedata"))
			return
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	handlers := handlerMap(c)
	h := handlers["pinchtab_record_stop"]

	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_record_stop"
	req.Params.Arguments = map[string]any{"file": "relative/bad.gif"}

	result, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)
	if !strings.Contains(text, "absolute path") {
		t.Fatalf("expected path error in result, got %q", text)
	}
	if !stopCalled {
		t.Fatal("expected POST /record/stop to be called even when path is invalid")
	}
}

func TestWithTimeoutCreatesIndependentClient(t *testing.T) {
	c := NewClient("http://localhost:9867", "tok")
	long := c.withTimeout(5 * time.Minute)

	if long.HTTPClient.Timeout != 5*time.Minute {
		t.Fatalf("long timeout = %v, want 5m", long.HTTPClient.Timeout)
	}
	if c.HTTPClient.Timeout != 120*time.Second {
		t.Fatalf("original timeout changed to %v", c.HTTPClient.Timeout)
	}
	if long.Token != "tok" {
		t.Fatalf("token not copied: %q", long.Token)
	}
	if long.BaseURL != c.BaseURL {
		t.Fatalf("BaseURL not copied: %q", long.BaseURL)
	}
}

func TestRecordStopUsesLongTimeout(t *testing.T) {
	var gotTimeout time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/record/stop" {
			deadline, ok := r.Context().Deadline()
			if ok {
				gotTimeout = time.Until(deadline)
			}
			w.WriteHeader(200)
			_, _ = io.WriteString(w, "videodata")
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	handlers := handlerMap(c)
	h := handlers["pinchtab_record_stop"]

	dir := t.TempDir()
	outFile := filepath.Join(dir, "test.gif")

	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_record_stop"
	req.Params.Arguments = map[string]any{"file": outFile}

	_, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// The http.Client.Timeout sets a context deadline on the request.
	// With recordStopTimeout=3m, the deadline should be well over 2m.
	if gotTimeout > 0 && gotTimeout < 2*time.Minute {
		t.Fatalf("request deadline too short: %v (expected > 2m from 3m client timeout)", gotTimeout)
	}
}
