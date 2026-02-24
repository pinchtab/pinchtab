//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

// T1: Readability mode
func TestText_Readability(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/text")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	if !strings.Contains(s, "Example Domain") {
		t.Error("expected 'Example Domain' in text output")
	}
}

// T2: Raw mode
func TestText_Raw(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/text?mode=raw")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	if !strings.Contains(s, "Example Domain") {
		t.Error("expected 'Example Domain' in raw text")
	}
}

// T3: Text with specific tabId
func TestText_WithTabId(t *testing.T) {
	// Create first tab and navigate to example.com
	code, body := httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "https://example.com",
	})
	if code != 200 {
		t.Skip("could not create first tab")
	}
	var tab1Data map[string]any
	_ = json.Unmarshal(body, &tab1Data)
	tab1ID, ok := tab1Data["tabId"].(string)
	if !ok || tab1ID == "" {
		t.Skip("no tabId in response")
	}

	// Create second tab and navigate to httpbin.org
	code, body = httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "https://httpbin.org",
	})
	if code != 200 {
		t.Skip("could not create second tab")
	}
	var tab2Data map[string]any
	_ = json.Unmarshal(body, &tab2Data)
	tab2ID, ok := tab2Data["tabId"].(string)
	if !ok || tab2ID == "" {
		t.Skip("no tabId in response")
	}

	// Get text from tab2 (should contain httpbin content)
	code, body = httpGet(t, "/text?tabId="+tab2ID)
	if code != 200 {
		t.Fatalf("expected 200 for /text?tabId=%s, got %d", tab2ID, code)
	}
	text := string(body)
	// httpbin.org should have "httpbin" in its content
	if !strings.Contains(text, "httpbin") && !strings.Contains(text, "HTTP") {
		t.Error("expected httpbin-related content in tab2 text")
	}
}

// T4: Text with non-existent tabId
func TestText_NoTab(t *testing.T) {
	code, _ := httpGet(t, "/text?tabId=nonexistent_xyz")
	if code == 200 {
		t.Errorf("expected error (400/404) for non-existent tab, got %d", code)
	}
}
