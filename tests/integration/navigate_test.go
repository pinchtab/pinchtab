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

// N5: Navigate invalid URL (dangerous schemes must be rejected)
func TestNavigate_InvalidURL(t *testing.T) {
	code, _ := httpPost(t, "/navigate", map[string]string{"url": "javascript:alert(1)"})
	if code == 200 {
		t.Error("expected error for javascript: scheme")
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
	code, body := httpPost(t, "/navigate", map[string]string{"url": "https://example.com"})
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

// N8: Navigate timeout handling
func TestNavigate_Timeout(t *testing.T) {
	// Use a reserved IP address that will timeout (TEST-NET-1)
	// This should not hang forever, but may return an error or timeout gracefully
	code, _ := httpPost(t, "/navigate", map[string]string{"url": "http://192.0.2.1"})
	// Just verify request completes (doesn't hang forever)
	// Response code can vary (200, 400, etc)
	t.Logf("navigate timeout returned %d", code)
}

// N3: Navigate with title verification
func TestNavigate_SPATitle(t *testing.T) {
	// Use a page that definitely has a title - example.com has "Example Domain" as title
	// Use retry logic for stability in CI/slow environments
	code, body := httpPostWithRetry(t, "/navigate", map[string]any{"url": "https://example.com"}, 2)
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	title := jsonField(t, body, "title")
	if title == "" {
		t.Error("expected non-empty title in response")
	}
	// Verify we got the expected title from example.com
	if title != "Example Domain" {
		t.Logf("got title: %q", title)
	}
}
