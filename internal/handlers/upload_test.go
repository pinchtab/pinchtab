package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
	"github.com/pinchtab/pinchtab/internal/selector"
)

type uploadLockBridge struct {
	mockBridge
	lock *bridge.LockInfo
}

func (m *uploadLockBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	return m.lock
}

type recordingUploadBridge struct {
	mockBridge
	selector      string
	nodeID        int64
	attachedPaths []string
	refCache      *bridge.RefCache
	gotRefCache   *bridge.RefCache
	gotFrameID    string
}

func (m *recordingUploadBridge) GetRefCache(string) *bridge.RefCache { return m.refCache }

func (m *recordingUploadBridge) ResolveSelectorToNodeID(_ context.Context, sel string, refCache *bridge.RefCache, frameID string) (int64, error) {
	m.selector = sel
	m.gotRefCache = refCache
	m.gotFrameID = frameID
	if parsed := selector.Parse(sel); parsed.Kind == selector.KindRef {
		if refCache != nil {
			if target, ok := refCache.Lookup(parsed.Value); ok {
				return target.BackendNodeID, nil
			}
		}
		return 0, fmt.Errorf("ref %s not in snapshot cache", parsed.Value)
	}
	return 42, nil
}

func (m *recordingUploadBridge) SetFileInputFiles(_ context.Context, nodeID int64, paths []string) error {
	m.nodeID = nodeID
	m.attachedPaths = append([]string(nil), paths...)
	for _, path := range m.attachedPaths {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("staged file was not readable before attach: %w", err)
		}
	}
	return nil
}

type failingSelectorUploadBridge struct {
	mockBridge
}

func (failingSelectorUploadBridge) ResolveSelectorToNodeID(context.Context, string, *bridge.RefCache, string) (int64, error) {
	return 0, fmt.Errorf("no element matches selector")
}

// A selector that doesn't resolve to a file input is a client error (4xx),
// not a 500 — consistent with the other element-targeting handlers.
func TestHandleUpload_SelectorNotFoundIs404(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(&failingSelectorUploadBridge{}, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unresolved selector, got %d: %s", w.Code, w.Body.String())
	}
}

type wrongElementUploadBridge struct {
	recordingUploadBridge
}

func (*wrongElementUploadBridge) SetFileInputFiles(context.Context, int64, []string) error {
	return fmt.Errorf("%w: target <button type=\"submit\"> is not an <input type=file> element", bridge.ErrInvalidActionRequest)
}

func TestHandleUpload_NonFileElementIs400InvalidActionRequest(t *testing.T) {
	h := New(&wrongElementUploadBridge{}, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"selector":"#send","files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("upload onto a non-file element answered %d, want 400: %s", w.Code, w.Body.String())
	}
	var body struct {
		Code      string `json:"code"`
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != "invalid_action_request" || body.Retryable {
		t.Fatalf("got code %q retryable %v, want invalid_action_request and not retryable", body.Code, body.Retryable)
	}
	if !strings.Contains(body.Error, `<button type="submit">`) {
		t.Fatalf("error %q does not name the element it hit", body.Error)
	}
}

func fileInputsBridge(candidates string) *recordingUploadBridge {
	rec := &recordingUploadBridge{}
	rec.evaluateFn = func(_ string, result any) error {
		return json.Unmarshal([]byte(candidates), result)
	}
	return rec
}

func uploadWithoutSelector(t *testing.T, rec *recordingUploadBridge) *httptest.ResponseRecorder {
	t.Helper()
	h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	return w
}

func TestHandleUpload_NoSelectorWithOneFileInputAttaches(t *testing.T) {
	rec := fileInputsBridge(`[{"label":"CV","selector":"#cv"}]`)
	w := uploadWithoutSelector(t, rec)

	if w.Code != http.StatusOK {
		t.Fatalf("single file input did not upload: %d %s", w.Code, w.Body.String())
	}
	if rec.selector != "input[type=file]" || len(rec.attachedPaths) != 1 {
		t.Fatalf("resolved %q and attached %d files, want the default selector and 1 file", rec.selector, len(rec.attachedPaths))
	}
}

func TestHandleUpload_NoSelectorWithTwoFileInputsRefusesAndListsThem(t *testing.T) {
	rec := fileInputsBridge(`[{"label":"CV","selector":"#cv"},{"label":"Photo","selector":"#photo"}]`)
	w := uploadWithoutSelector(t, rec)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("two file inputs and no selector answered %d, want 400: %s", w.Code, w.Body.String())
	}
	var body struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Details struct {
			Candidates []fileInputCandidate `json:"candidates"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != "upload_ambiguous" {
		t.Fatalf("got code %q, want upload_ambiguous", body.Code)
	}
	for _, want := range []string{"CV", "#cv", "Photo", "#photo"} {
		if !strings.Contains(body.Error, want) {
			t.Fatalf("error %q does not list candidate %q", body.Error, want)
		}
	}
	if len(body.Details.Candidates) != 2 || body.Details.Candidates[1].Selector != "#photo" {
		t.Fatalf("details.candidates = %+v, want both inputs with their selectors", body.Details.Candidates)
	}
	if rec.selector != "" || rec.attachedPaths != nil {
		t.Fatalf("refused upload still resolved %q and attached %v", rec.selector, rec.attachedPaths)
	}
}

func TestHandleUpload_ExplicitSelectorSkipsTheFileInputCensus(t *testing.T) {
	rec := fileInputsBridge(`[{"label":"CV","selector":"#cv"},{"label":"Photo","selector":"#photo"}]`)
	h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"selector":"#photo","files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusOK || rec.evaluateCalls != 0 {
		t.Fatalf("explicit selector answered %d after %d evaluations, want 200 and none: %s", w.Code, rec.evaluateCalls, w.Body.String())
	}
}

func TestHandleUpload_ResolvesRefSelector(t *testing.T) {
	rec := &recordingUploadBridge{refCache: &bridge.RefCache{Refs: map[string]int64{"e0": 4242}}}
	h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"selector":"e0","files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ref selector e0 did not upload: %d %s", w.Code, w.Body.String())
	}
	if rec.gotRefCache != rec.refCache {
		t.Fatalf("handler did not forward the tab's RefCache (got %v), so a ref cannot resolve", rec.gotRefCache)
	}
	if rec.nodeID != 4242 {
		t.Fatalf("ref e0 resolved to node %d, want 4242 from the cache", rec.nodeID)
	}
}

func TestHandleUpload_ResolvesFrameScopedSelector(t *testing.T) {
	rec := &recordingUploadBridge{}
	rec.SetFrameScope("tab1", bridge.FrameScope{FrameID: "FRAME-7"})
	h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"selector":"#f","files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("frame-scoped selector did not upload: %d %s", w.Code, w.Body.String())
	}
	if rec.gotFrameID != "FRAME-7" {
		t.Fatalf("handler resolved the selector in frame %q, want FRAME-7 — an in-frame input is unreachable otherwise", rec.gotFrameID)
	}
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
	for i := range config.DefaultUploadMaxFiles + 1 {
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

func TestHandleUpload_BodyOverTheConfiguredRequestCapIs413(t *testing.T) {
	body := `{"files":["` + strings.Repeat("a", 128) + `"]}`
	for _, tc := range []struct {
		maxRequestBytes int
		want413         bool
	}{
		{maxRequestBytes: 64, want413: true},
		{maxRequestBytes: 4096, want413: false},
	} {
		h := New(&mockBridge{}, &config.RuntimeConfig{AllowUpload: true, UploadMaxRequestBytes: tc.maxRequestBytes}, nil, nil, nil)
		req := httptest.NewRequest("POST", "/upload", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.HandleUpload(w, req)
		if got413 := w.Code == http.StatusRequestEntityTooLarge; got413 != tc.want413 {
			t.Errorf("cap %d: status = %d, want 413 = %v: %s", tc.maxRequestBytes, w.Code, tc.want413, w.Body.String())
		}
	}
}

func TestDecodeFileData_DataURL(t *testing.T) {
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
	if data[0] != 0x89 || data[1] != 'P' {
		t.Error("expected PNG magic bytes")
	}
}

func TestDecodeFileData_RawBase64(t *testing.T) {
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
	origCleanupStagedUploadDirAfter := cleanupStagedUploadDirAfter
	t.Cleanup(func() {
		cleanupStagedUploadDirAfter = origCleanupStagedUploadDirAfter
	})

	var cleanupDir string
	var cleanupAfter time.Duration
	cleanupStagedUploadDirAfter = func(dir string, after time.Duration) {
		cleanupDir = dir
		cleanupAfter = after
		_ = os.RemoveAll(dir)
	}

	tmpDir := t.TempDir()
	uploadBridge := &recordingUploadBridge{}
	h := New(uploadBridge, &config.RuntimeConfig{AllowUpload: true, StateDir: tmpDir}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload?tabId=t1", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if uploadBridge.selector != "input[type=file]" {
		t.Fatalf("selector = %q, want default file input", uploadBridge.selector)
	}
	if uploadBridge.nodeID != 42 {
		t.Fatalf("nodeID = %d, want 42", uploadBridge.nodeID)
	}
	if len(uploadBridge.attachedPaths) != 1 {
		t.Fatalf("attached paths = %d, want 1", len(uploadBridge.attachedPaths))
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

	h := New(&mockBridge{}, &config.RuntimeConfig{StateDir: tmpDir}, nil, nil, nil)
	h.StartBackgroundCleanup()

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

// stagedNames runs one upload and returns the basenames CDP was handed, which is
// exactly what the page reads file.name from.
func stagedNames(t *testing.T, body string) []string {
	t.Helper()

	rec := &recordingUploadBridge{}
	h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/upload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	names := make([]string, 0, len(rec.attachedPaths))
	for _, p := range rec.attachedPaths {
		names = append(names, filepath.Base(p))
	}
	return names
}

// The defect: every file reached the page as upload-<i>.bin, so a form gating on
// `accept=".csv"` rejected it. The staged basename IS the page-visible
// file.name (asserted against a real browser in
// internal/bridge/upload_filename_test.go), so this is that name.
func TestHandleUpload_KeepsTheSuppliedFileName(t *testing.T) {
	names := stagedNames(t, `{"files":["aGVsbG8="],"fileNames":["data.csv"]}`)

	if len(names) != 1 || names[0] != "data.csv" {
		t.Fatalf("page would see %v, want [data.csv]", names)
	}
}

// Text formats are the ones upload forms validate by extension and the ones
// content sniffing can never identify — no magic bytes exist for .csv, .json or
// .txt. A supplied name is the only way they can arrive correctly named.
func TestHandleUpload_NamesSniffingCannotProduce(t *testing.T) {
	for _, name := range []string{"notes.txt", "export.json", "sheet.csv", "page.html", "readme.md"} {
		got := stagedNames(t, fmt.Sprintf(`{"files":["aGVsbG8="],"fileNames":[%q]}`, name))
		if len(got) != 1 || got[0] != name {
			t.Errorf("page would see %v, want [%s]", got, name)
		}
	}
}

// Unchanged behaviour for a caller posting a bare blob: no name, so the
// extension is sniffed from content and the generated basename stands.
func TestHandleUpload_FallsBackToSniffedNameWithoutASuppliedName(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})

	for _, tc := range []struct{ name, body, want string }{
		{"no fileNames at all", `{"files":["` + png + `"]}`, "upload-0.png"},
		{"empty name", `{"files":["` + png + `"],"fileNames":[""]}`, "upload-0.png"},
		{"unrecognised content", `{"files":["aGVsbG8="]}`, "upload-0.bin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stagedNames(t, tc.body)
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("staged %v, want [%s]", got, tc.want)
			}
		})
	}
}

// A shorter fileNames list must not shift names onto the wrong files or drop the
// remaining uploads: the unnamed tail falls back to sniffing, index by index.
func TestHandleUpload_ShortFileNamesListNamesOnlyItsOwnFiles(t *testing.T) {
	names := stagedNames(t, `{"files":["aGVsbG8=","aGVsbG8=","aGVsbG8="],"fileNames":["first.csv"]}`)

	want := []string{"first.csv", "upload-1.bin", "upload-2.bin"}
	if len(names) != len(want) {
		t.Fatalf("staged %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("file %d staged as %q, want %q", i, names[i], want[i])
		}
	}
}

// The case that could not break before the index moved out of the filename: two
// files sent under ONE name in one request. Both must reach the page under that
// name — a de-duplicating rename would corrupt the very thing this fixes, and a
// shared path would let os.WriteFile truncate the first file away.
func TestHandleUpload_DuplicateNamesInOneRequestStayDistinctFiles(t *testing.T) {
	rec := &recordingUploadBridge{}
	h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: t.TempDir(), ActionTimeout: time.Second}, nil, nil, nil)

	first := base64.StdEncoding.EncodeToString([]byte("first file"))
	second := base64.StdEncoding.EncodeToString([]byte("second file, longer"))
	body := fmt.Sprintf(`{"files":[%q,%q],"fileNames":["data.csv","data.csv"]}`, first, second)

	req := httptest.NewRequest("POST", "/upload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	if len(rec.attachedPaths) != 2 {
		t.Fatalf("attached %d paths, want 2 — a shared path silently loses a file", len(rec.attachedPaths))
	}
	if rec.attachedPaths[0] == rec.attachedPaths[1] {
		t.Fatalf("both files staged at %q; the second truncated the first", rec.attachedPaths[0])
	}
	for i, p := range rec.attachedPaths {
		if got := filepath.Base(p); got != "data.csv" {
			t.Errorf("file %d staged as %q, want data.csv — a de-duplicating rename defeats the fix", i, got)
		}
	}
	// Distinct paths are only worth anything if the CONTENT survived separately.
	firstBytes, err := os.ReadFile(rec.attachedPaths[0])
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(rec.attachedPaths[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != "first file" || string(secondBytes) != "second file, longer" {
		t.Errorf("staged contents = %q / %q, want the two distinct payloads", firstBytes, secondBytes)
	}
}

// Containment as a property of the staged NAME, asserted precisely rather than
// tolerantly: each traversal attempt must land on a named result inside the
// per-file directory. An earlier version of this test accepted "or the request
// is refused", which made it blind to whether the reduction happened at all.
// This surface did not exist before a caller-supplied string became part of a
// filesystem path.
func TestHandleUpload_SuppliedNameCannotEscapeTheStagingDirectory(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"../escape.txt", "escape.txt"},
		{"../../../../etc/passwd", "passwd"},
		{"sub/dir/nested.txt", "nested.txt"},
		{"/etc/passwd", "passwd"},
		{"..", "upload-0.bin"},
		{".", "upload-0.bin"},
		{"/", "upload-0.bin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingUploadBridge{}
			stateDir := t.TempDir()
			h := New(rec, &config.RuntimeConfig{AllowUpload: true, StateDir: stateDir, ActionTimeout: time.Second}, nil, nil, nil)

			body := fmt.Sprintf(`{"files":["aGVsbG8="],"fileNames":[%q]}`, tc.name)
			req := httptest.NewRequest("POST", "/upload", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleUpload(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 — a browser reduces such a name rather than refusing: %s", w.Code, w.Body.String())
			}
			if len(rec.attachedPaths) != 1 {
				t.Fatalf("attached %v, want exactly one staged file", rec.attachedPaths)
			}
			if got := filepath.Base(rec.attachedPaths[0]); got != tc.want {
				t.Errorf("staged as %q, want %q", got, tc.want)
			}

			staged, err := filepath.EvalSymlinks(rec.attachedPaths[0])
			if err != nil {
				t.Fatal(err)
			}
			root, err := filepath.EvalSymlinks(filepath.Join(stateDir, uploadSandboxDirName))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(staged, root+string(filepath.Separator)) {
				t.Fatalf("staged %q escaped the upload root %q", staged, root)
			}
		})
	}
}

// The reduction rule, driven directly, so every branch has a named case — the
// handler test above can only reach the ones an upload can carry.
func TestStagedUploadNameIsAlwaysOneSafeElement(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"data.csv", "data.csv"},
		{"  spaced.csv  ", "spaced.csv"},
		{"../escape.txt", "escape.txt"},
		{"a/b/c.json", "c.json"},
		// A Windows caller reaches a POSIX server with no '/' in its name at all, so
		// reduction cannot be delegated to the host's separator.
		{`C:\Users\me\data.csv`, "data.csv"},
		{`..\..\etc\passwd`, "passwd"},
		{`sub\dir\nested.txt`, "nested.txt"},
		{`trailing\`, "upload-3.bin"},
		{"trailing/", "upload-3.bin"},
		{"", "upload-3.bin"},
		{"   ", "upload-3.bin"},
		{".", "upload-3.bin"},
		{"..", "upload-3.bin"},
		{"/", "upload-3.bin"},
		{"with\x00null", "upload-3.bin"},
	} {
		got := stagedUploadName(tc.name, 3, ".bin")
		if got != tc.want {
			t.Errorf("stagedUploadName(%q) = %q, want %q", tc.name, got, tc.want)
		}
		if got == "" || got == "." || got == ".." || strings.ContainsAny(got, `/\`) || strings.ContainsRune(got, filepath.Separator) {
			t.Errorf("stagedUploadName(%q) = %q, which is not a single safe path element", tc.name, got)
		}
	}
}

// Precedence: a name wins over the sniffed content type even when the two
// disagree. A browser sends the user's filename regardless of content and lets
// the receiving page decide whether to trust it; a consistency check here would
// diverge from that and break legitimate mislabelled-but-intended uploads.
func TestHandleUpload_SuppliedNameWinsOverSniffedContent(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})

	names := stagedNames(t, fmt.Sprintf(`{"files":[%q],"fileNames":["actually-a-png.csv"]}`, png))
	if len(names) != 1 || names[0] != "actually-a-png.csv" {
		t.Fatalf("staged %v, want [actually-a-png.csv]", names)
	}
}
