//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// N1: Basic navigate
func TestNavigate_Basic(t *testing.T) {
	code, body := httpPost(t, "/navigate", map[string]string{"url": "https://example.com"})
	if code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}
	title := jsonField(t, body, "title")
	if title != "Example Domain" {
		t.Errorf("expected title 'Example Domain', got %q", title)
	}
}

// N5: Navigate invalid URL
func TestNavigate_InvalidURL(t *testing.T) {
	code, _ := httpPost(t, "/navigate", map[string]string{"url": "not-a-url"})
	if code == 200 {
		t.Error("expected error for invalid URL")
	}
}

// N6: Navigate missing URL
func TestNavigate_MissingURL(t *testing.T) {
	code, _ := httpPost(t, "/navigate", map[string]string{})
	if code != 400 {
		t.Errorf("expected 400, got %d", code)
	}
}

// N7: Navigate bad JSON
func TestNavigate_BadJSON(t *testing.T) {
	code, _ := httpPostRaw(t, "/navigate", "{broken")
	if code != 400 {
		t.Errorf("expected 400, got %d", code)
	}
}

// N2: Navigate returns title
func TestNavigate_ReturnsTitle(t *testing.T) {
	code, body := httpPost(t, "/navigate", map[string]string{"url": "https://httpbin.org/html"})
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	title := jsonField(t, body, "title")
	if title == "" {
		t.Error("expected non-empty title")
	}
}

// N4: Navigate with newTab
func TestNavigate_NewTab(t *testing.T) {
	code, body := httpPost(t, "/navigate", map[string]any{
		"url":    "https://example.com",
		"newTab": true,
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["tabId"] == nil || m["tabId"] == "" {
		t.Error("expected tabId in response for newTab")
	}
}
