package handlers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type uploadLockBridge struct {
	mockBridge
	lock *bridge.LockInfo
}

func (m *uploadLockBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	return m.lock
}

func TestHandleUpload_BadJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", w.Code)
	}
}

func TestHandleUpload_EmptyPaths(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	body := `{"selector": "input[type=file]"}`
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty paths, got %d", w.Code)
	}
}

func TestHandleUpload_NonexistentPath(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	body := `{"selector": "input[type=file]", "paths": ["/tmp/nonexistent-file-12345.jpg"]}`
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for nonexistent path, got %d", w.Code)
	}
}

func TestHandleUpload_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)

	tests := []struct {
		name string
		path string
	}{
		{"dotdot traversal", "../etc/passwd"},
		{"absolute outside", "/etc/passwd"},
		{"hidden traversal", "uploads/../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"selector": "input[type=file]", "paths": [%q]}`, tt.path)
			req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleUpload(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for traversal path %q, got %d", tt.path, w.Code)
			}
		})
	}
}

func TestHandleUpload_AllowsUploadSandboxPath(t *testing.T) {
	tmpDir := t.TempDir()
	uploadsDir := filepath.Join(tmpDir, "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadsDir, "test.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)
	body := `{"selector": "input[type=file]", "paths": ["uploads/test.txt"]}`
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected sandbox path validation to pass and tab lookup to fail, got %d", w.Code)
	}
}

func TestHandleUpload_RejectsSymlinkedUploadSandboxPath(t *testing.T) {
	tmpDir := t.TempDir()
	uploadsDir := filepath.Join(tmpDir, "uploads")
	outsideDir := t.TempDir()
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(uploadsDir, "linked")); err != nil {
		t.Skipf("symlink unsupported in test environment: %v", err)
	}

	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)
	body := `{"selector": "input[type=file]", "paths": ["uploads/linked/secret.txt"]}`
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected symlinked sandbox path to be rejected, got %d", w.Code)
	}
}

func TestHandleUpload_RejectsTooManyFiles(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)

	var body bytes.Buffer
	body.WriteString(`{"files":[`)
	for i := 0; i < config.DefaultUploadMaxFiles+1; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`"aGVsbG8="`)
	}
	body.WriteString(`]}`)

	req := httptest.NewRequest("POST", "/upload", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too many files, got %d", w.Code)
	}
}

func TestHandleUpload_RejectsDecodedFileTooLarge(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	large := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), config.DefaultUploadMaxFileBytes+1))
	body := fmt.Sprintf(`{"files": ["%s"]}`, large)
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized decoded file, got %d", w.Code)
	}
}

func TestHandleUpload_UsesConfiguredUploadLimits(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		AllowUpload:         true,
		UploadMaxFiles:      1,
		UploadMaxFileBytes:  4,
		UploadMaxTotalBytes: 4,
	}, nil, nil, nil)

	body := `{"files":["aGVsbG8="]}`
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for configured file size limit, got %d", w.Code)
	}
}

func TestHandleUpload_TabLocked(t *testing.T) {
	h := New(&uploadLockBridge{
		lock: &bridge.LockInfo{
			Owner:     "alice",
			ExpiresAt: time.Now().Add(time.Minute),
		},
	}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/upload?tabId=tab1", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423 for locked tab, got %d", w.Code)
	}
}

func TestHandleTabUpload_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//upload", bytes.NewReader([]byte(`{"selector":"input[type=file]"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabUpload_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/upload", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.SetPathValue("id", "tab_abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabUpload(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpload_BodyTooLarge(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true}, nil, nil, nil)
	// Create a body larger than 10MB
	bigBody := make([]byte, 11<<20) // 11MB
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized body, got %d", w.Code)
	}
}

func TestDecodeFileData_DataURL(t *testing.T) {
	// 1x1 red PNG as data URL
	input := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	data, ext, err := decodeFileData(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".png" {
		t.Errorf("expected .png, got %s", ext)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
	// Check PNG magic bytes
	if data[0] != 0x89 || data[1] != 'P' {
		t.Error("expected PNG magic bytes")
	}
}

func TestDecodeFileData_RawBase64(t *testing.T) {
	// 1x1 red PNG as raw base64
	input := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	data, ext, err := decodeFileData(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".png" {
		t.Errorf("expected .png (sniffed), got %s", ext)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestDecodeFileData_InvalidBase64(t *testing.T) {
	_, _, err := decodeFileData("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestMimeToExt(t *testing.T) {
	tests := []struct {
		mime string
		ext  string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"application/pdf", ".pdf"},
		{"text/plain", ".txt"},
		{"application/octet-stream", ".bin"},
	}
	for _, tt := range tests {
		if got := mimeToExt(tt.mime); got != tt.ext {
			t.Errorf("mimeToExt(%q) = %q, want %q", tt.mime, got, tt.ext)
		}
	}
}

func TestSniffExt(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		ext  string
	}{
		{"png", []byte{0x89, 'P', 'N', 'G'}, ".png"},
		{"jpg", []byte{0xFF, 0xD8, 0x00, 0x00}, ".jpg"},
		{"gif", []byte("GIF89a"), ".gif"},
		{"pdf", []byte("%PDF-1.4"), ".pdf"},
		{"unknown", []byte{0x00, 0x01, 0x02, 0x03}, ".bin"},
		{"short", []byte{0x00}, ".bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sniffExt(tt.data); got != tt.ext {
				t.Errorf("sniffExt() = %q, want %q", got, tt.ext)
			}
		})
	}
}

func TestHandleUpload_Disabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(`{"paths":["/tmp/test.png"]}`)))
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403 when upload disabled, got %d", w.Code)
	}
}
