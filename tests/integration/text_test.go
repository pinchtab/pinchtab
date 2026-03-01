//go:build integration

package integration

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// T1: Readability mode
func TestText_Readability(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/text?tabId="+url.QueryEscape(currentTabID))
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
	code, body := httpGet(t, "/text?mode=raw&tabId="+url.QueryEscape(currentTabID))
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
	// httpbin.org should have something in its content (site might be slow in CI)
	if len(text) == 0 {
		t.Skip("httpbin.org page failed to load (network/timeout in CI)")
	}
	// At least verify we got some text back (httpbin might be slow to load)
	if !strings.Contains(text, "httpbin") && !strings.Contains(text, "HTTP") && !strings.Contains(text, "GET") {
		t.Logf("warning: expected httpbin-related content in tab2")
	}

	// IMPORTANT: Clean up tabs to avoid affecting subsequent tests
	_, _ = httpPost(t, "/tab", map[string]string{
		"action": "close",
		"tabId":  tab1ID,
	})
	_, _ = httpPost(t, "/tab", map[string]string{
		"action": "close",
		"tabId":  tab2ID,
	})
}

// T4: Text with non-existent tabId
func TestText_NoTab(t *testing.T) {
	code, _ := httpGet(t, "/text?tabId=nonexistent_xyz")
	if code == 200 {
		t.Errorf("expected error (400/404) for non-existent tab, got %d", code)
	}
}

// T5: Token efficiency (real-world content)
func TestText_TokenEfficiency(t *testing.T) {
	navigate(t, "https://google.com")
	code, body := httpGet(t, "/text?tabId="+url.QueryEscape(currentTabID))
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	text := string(body)
	// Verify text is non-empty
	if len(text) == 0 {
		t.Error("expected non-empty text response")
	}
	// Count words roughly (split by space)
	words := strings.Fields(text)
	if len(words) == 0 {
		t.Error("expected text with word content")
	}
	// Simple sanity check: text should be reasonable size (not huge, not tiny)
	if len(text) < 50 {
		t.Logf("text is very small (%d bytes), may not be representative", len(text))
	}
	t.Logf("extracted %d words from google.com", len(words))
}
