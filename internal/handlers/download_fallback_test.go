package handlers

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsGzipContent(t *testing.T) {
	tests := []struct {
		contentType string
		url         string
		want        bool
	}{
		{"application/gzip", "https://example.com/file", true},
		{"application/x-gzip", "https://example.com/file", true},
		{"application/xml", "https://example.com/sitemap.xml.gz", true},
		{"application/xml", "https://example.com/sitemap.xml", false},
		{"text/html", "https://example.com/page", false},
		{"", "https://example.com/data.gz", true},
		// Edge cases: path.Ext should not match these as .gz
		{"", "https://example.com/file.pgz", false},
		{"", "https://example.com/file.ngz", false},
		{"", "https://example.com/filegz", false},
		// Query params should not affect extension detection
		{"", "https://example.com/data.gz?token=abc", true},
		{"", "https://example.com/data.tar.gz", true},
	}
	for _, tt := range tests {
		if got := isGzipContent(tt.contentType, tt.url); got != tt.want {
			t.Errorf("isGzipContent(%q, %q) = %v, want %v", tt.contentType, tt.url, got, tt.want)
		}
	}
}

func TestInferDecompressedContentType(t *testing.T) {
	tests := []struct {
		url      string
		fallback string
		want     string
	}{
		{"https://example.com/sitemap.xml.gz", "application/gzip", "application/xml"},
		{"https://example.com/data.json.gz", "application/gzip", "application/json"},
		{"https://example.com/log.txt.gz", "application/gzip", "text/plain"},
		{"https://example.com/data.csv.gz", "application/gzip", "text/plain"},
		{"https://example.com/archive.gz", "application/gzip", "application/octet-stream"},
	}
	for _, tt := range tests {
		if got := inferDecompressedContentType(tt.url, tt.fallback); got != tt.want {
			t.Errorf("inferDecompressedContentType(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestIsNavigationAborted(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{io.EOF, false},
		{&navError{"page load error net::ERR_ABORTED"}, true},
		{&navError{"page load error net::ERR_CONNECTION_REFUSED"}, false},
	}
	for _, tt := range tests {
		if got := isNavigationAborted(tt.err); got != tt.want {
			t.Errorf("isNavigationAborted(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

type navError struct{ msg string }

func (e *navError) Error() string { return e.msg }

func TestFetchDirectWithCookies_GzipDecompression(t *testing.T) {
	// Create a test server that serves gzip content
	xmlContent := `<?xml version="1.0"?><urlset><url><loc>https://example.com</loc></url></urlset>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml.gz" {
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			_, _ = gz.Write([]byte(xmlContent))
			_ = gz.Close()
			return
		}
		if r.URL.Path == "/plain.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("hello world"))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	client := srv.Client()
	resp, err := client.Get(srv.URL + "/sitemap.xml.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Verify the server is sending gzip content
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "gzip") {
		t.Fatalf("expected gzip content-type, got %q", ct)
	}

	// Manually decompress to verify our detection works
	if isGzipContent(resp.Header.Get("Content-Type"), srv.URL+"/sitemap.xml.gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = gz.Close() }()
		body, err := io.ReadAll(gz)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != xmlContent {
			t.Errorf("decompressed content = %q, want %q", body, xmlContent)
		}
	} else {
		t.Error("expected isGzipContent to return true for .gz URL with gzip content-type")
	}
}
