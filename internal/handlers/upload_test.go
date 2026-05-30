package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// Regression: decoded base64 uploads must persist past the handler so the browser
// can read them LAZILY at form-submit time (a separate later request). They are
// kept on success and removed only on a pre-attach failure; never written into
// the process working directory.
func TestHandleUpload_StagedFilesCleanedOnFailureNotCWD(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	body := fmt.Sprintf(`{"selector":"input[type=file]","files":[%q]}`, png)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (failTab) after decode, got %d", w.Code)
	}
	// The decode succeeded but the upload never attached to a tab → staged dir
	// must be cleaned up.
	entries, _ := os.ReadDir(filepath.Join(tmpDir, "uploads"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "pinchtab-upload-") {
			t.Fatalf("staged upload dir leaked on failure: %s", e.Name())
		}
	}
	// Must never create an "uploads" dir in the process working directory.
	if _, err := os.Stat("uploads"); err == nil {
		_ = os.RemoveAll("uploads")
		t.Fatalf("upload handler created ./uploads in the working directory")
	}
}

func TestHandleUpload_SuccessSchedulesStagedCleanup(t *testing.T) {
	origSetUploadFileInputFiles := setUploadFileInputFiles
	origCleanupStagedUploadDirAfter := cleanupStagedUploadDirAfter
	t.Cleanup(func() {
		setUploadFileInputFiles = origSetUploadFileInputFiles
		cleanupStagedUploadDirAfter = origCleanupStagedUploadDirAfter
	})

	var attachedPaths []string
	setUploadFileInputFiles = func(_ context.Context, selector string, paths []string) error {
		if selector != "input[type=file]" {
			t.Fatalf("selector = %q, want default file input", selector)
		}
		attachedPaths = append([]string(nil), paths...)
		for _, path := range attachedPaths {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("staged file was not readable before attach: %v", err)
			}
		}
		return nil
	}

	var cleanupDir string
	var cleanupAfter time.Duration
	cleanupStagedUploadDirAfter = func(dir string, after time.Duration) {
		cleanupDir = dir
		cleanupAfter = after
		_ = os.RemoveAll(dir)
	}

	tmpDir := t.TempDir()
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(attachedPaths) != 1 {
		t.Fatalf("attached paths = %d, want 1", len(attachedPaths))
	}
	if cleanupDir == "" {
		t.Fatal("successful upload did not schedule staged cleanup")
	}
	if cleanupAfter != uploadStagedRetention {
		t.Fatalf("cleanup delay = %s, want %s", cleanupAfter, uploadStagedRetention)
	}
	if !strings.HasPrefix(filepath.Base(cleanupDir), "pinchtab-upload-") {
		t.Fatalf("cleanup dir %q does not use staged upload prefix", cleanupDir)
	}
	if filepath.Dir(cleanupDir) != filepath.Join(tmpDir, "uploads") {
		t.Fatalf("cleanup dir %q not under state uploads dir", cleanupDir)
	}
	if _, err := os.Stat(cleanupDir); !os.IsNotExist(err) {
		t.Fatalf("scheduled cleanup did not remove staged dir in test: %v", err)
	}
}

func TestNewCleansStaleUploadStagingDirs(t *testing.T) {
	tmpDir := t.TempDir()
	uploadsDir := filepath.Join(tmpDir, "uploads")
	staleDir := filepath.Join(uploadsDir, "pinchtab-upload-stale")
	freshDir := filepath.Join(uploadsDir, "pinchtab-upload-fresh")
	userDir := filepath.Join(uploadsDir, "user-managed")

	for _, dir := range []string{staleDir, freshDir, userDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "upload-0.txt"), []byte("hello"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(staleDir, old, old); err != nil {
		t.Fatal(err)
	}

	_ = New(&mockBridge{}, &config.RuntimeConfig{StateDir: tmpDir}, nil, nil, nil)

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		_, err := os.Stat(staleDir)
		if os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stale upload staging dir was not cleaned: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("fresh upload staging dir should be kept: %v", err)
	}
	if _, err := os.Stat(userDir); err != nil {
		t.Fatalf("non-staged upload sandbox dir should be kept: %v", err)
	}
}
