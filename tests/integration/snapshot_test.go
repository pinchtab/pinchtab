//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

// S1: Basic snapshot
func TestSnapshot_Basic(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/snapshot")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	// Should have nodes or tree
	if m["nodes"] == nil && m["tree"] == nil && m["role"] == nil {
		t.Error("expected snapshot data (nodes, tree, or role)")
	}
}

// S2: Interactive filter
func TestSnapshot_Interactive(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/snapshot?filter=interactive")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	// example.com has a link "More information..."
	if !strings.Contains(s, "link") && !strings.Contains(s, "More information") {
		t.Log("warning: expected interactive elements in snapshot")
	}
}

// S4: Text format
func TestSnapshot_TextFormat(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/snapshot?format=text")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	// Text format should not be JSON
	if strings.HasPrefix(strings.TrimSpace(s), "{") || strings.HasPrefix(strings.TrimSpace(s), "[") {
		t.Error("text format should not return JSON")
	}
	if !strings.Contains(s, "Example Domain") {
		t.Error("expected 'Example Domain' in text snapshot")
	}
}

// S5: YAML format
func TestSnapshot_YAMLFormat(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/snapshot?format=yaml")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		t.Error("yaml format should not return JSON object")
	}
}

// S3: Depth filter
func TestSnapshot_Depth(t *testing.T) {
	navigate(t, "https://example.com")
	code, _ := httpGet(t, "/snapshot?depth=2")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
}

// S5 (compact): maxTokens
func TestSnapshot_MaxTokens(t *testing.T) {
	navigate(t, "https://example.com")
	code, _ := httpGet(t, "/snapshot?maxTokens=500")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
}

// S9: Snapshot with specific tabId
func TestSnapshot_WithTabId(t *testing.T) {
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

	// Get snapshot from tab2 (should contain httpbin content)
	code, body = httpGet(t, "/snapshot?tabId="+tab2ID)
	if code != 200 {
		t.Fatalf("expected 200 for /snapshot?tabId=%s, got %d", tab2ID, code)
	}
	snapshotText := string(body)
	// httpbin.org should have "httpbin" or "HTTP" in its snapshot
	if !strings.Contains(snapshotText, "httpbin") && !strings.Contains(snapshotText, "HTTP") {
		t.Error("expected httpbin-related content in tab2 snapshot")
	}
}

// S10: Snapshot with non-existent tabId
func TestSnapshot_NoTab(t *testing.T) {
	code, _ := httpGet(t, "/snapshot?tabId=nonexistent_xyz")
	if code == 200 {
		t.Errorf("expected error (400/404) for non-existent tab, got %d", code)
	}
}
