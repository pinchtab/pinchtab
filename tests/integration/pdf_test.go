//go:build integration

package integration

import (
	"encoding/base64"
	"os"
	"testing"
)

// PD1: PDF base64
func TestPDF_Base64(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/pdf")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	b64 := jsonField(t, body, "base64")
	if b64 == "" {
		t.Error("expected base64 field in response")
		return
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("invalid base64: %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		t.Error("decoded data is not a PDF")
	}
}

// PD2: PDF raw bytes
func TestPDF_Raw(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/pdf?raw=true")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(body) < 4 || string(body[:4]) != "%PDF" {
		t.Error("expected raw PDF data")
	}
}

// PD3: PDF save to file (uses default state dir)
func TestPDF_SaveFile(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/pdf?output=file")
	if code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}
	// Response should have a path field pointing to where it was saved
	path := jsonField(t, body, "path")
	if path == "" {
		t.Error("expected path field in response")
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Logf("file path: %s", path)
		t.Fatalf("file not created: %v", err)
	}
	if info.Size() < 100 {
		t.Error("PDF file too small")
	}
}

// PD5: PDF landscape
func TestPDF_Landscape(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/pdf?landscape=true&raw=true")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(body) < 4 || string(body[:4]) != "%PDF" {
		t.Error("expected valid PDF")
	}
}

// PD6: PDF scale
func TestPDF_Scale(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/pdf?scale=0.5&raw=true")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(body) < 4 || string(body[:4]) != "%PDF" {
		t.Error("expected valid PDF")
	}
}

// PD7: PDF no tab — tested implicitly (hard to close all tabs safely)
