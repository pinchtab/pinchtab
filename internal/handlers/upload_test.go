package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleUpload_BadJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", w.Code)
	}
}

func TestHandleUpload_EmptyPaths(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
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
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"selector": "input[type=file]", "paths": ["/tmp/nonexistent-file-12345.jpg"]}`
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for nonexistent path, got %d", w.Code)
	}
}

func TestHandleTabUpload_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//upload", bytes.NewReader([]byte(`{"selector":"input[type=file]"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabUpload_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
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
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
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
