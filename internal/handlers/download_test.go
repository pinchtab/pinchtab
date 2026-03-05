package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleDownload_MissingURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDownload_EmptyURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?url=", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty URL, got %d", w.Code)
	}
}

func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com/file.pdf", false},
		{"valid http", "http://example.com/page", false},
		{"file scheme", "file:///etc/passwd", true},
		{"ftp scheme", "ftp://example.com/file", true},
		{"data scheme", "data:text/html,hello", true},
		{"localhost", "http://localhost:8080/secret", true},
		{"loopback ipv4", "http://127.0.0.1/secret", true},
		{"loopback ipv6", "http://[::1]/secret", true},
		{"empty scheme", "://example.com", true},
		{"no scheme", "example.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDownloadURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestHandleDownload_SSRFBlocked(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	urls := []string{
		"file:///etc/passwd",
		"http://localhost:8080",
		"http://127.0.0.1/admin",
	}
	for _, u := range urls {
		req := httptest.NewRequest("GET", "/download?url="+u, nil)
		w := httptest.NewRecorder()
		h.HandleDownload(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for SSRF URL %q, got %d", u, w.Code)
		}
	}
}

func TestHandleTabDownload_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//download?url=https://example.com", nil)
	w := httptest.NewRecorder()
	h.HandleTabDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabDownload_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/download?url=https://example.com", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabDownload(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
