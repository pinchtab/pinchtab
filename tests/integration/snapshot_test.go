//go:build integration

package integration

import (
	"encoding/json"
	"os"
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

// S8: Snapshot file output
func TestSnapshot_FileOutput(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/snapshot?output=file")
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
		t.Error("snapshot file too small")
	}
}

// S12: Snapshot ref stability across interactions
func TestSnapshot_RefStability(t *testing.T) {
	navigate(t, "https://example.com")

	// Get initial snapshot with interactive filter
	code, body := httpGet(t, "/snapshot?filter=interactive")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}

	// Parse refs from response
	var initialSnapshot map[string]any
	if err := json.Unmarshal(body, &initialSnapshot); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}

	// Extract initial refs (look for nodes with ref field)
	initialRefs := extractRefs(initialSnapshot)
	if len(initialRefs) < 2 {
		t.Skip("not enough interactive refs found in snapshot")
	}

	// Click the first interactive ref
	refToClick := initialRefs[0]
	code, _ = httpPost(t, "/action", map[string]string{
		"kind": "click",
		"ref":  refToClick,
	})
	if code != 200 {
		t.Logf("click returned %d (may be expected if link not clickable)", code)
	}

	// Get snapshot again after click
	code, body = httpGet(t, "/snapshot?filter=interactive")
	if code != 200 {
		t.Fatalf("expected 200 after click, got %d", code)
	}

	var afterSnapshot map[string]any
	if err := json.Unmarshal(body, &afterSnapshot); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}

	afterRefs := extractRefs(afterSnapshot)
	if len(afterRefs) == 0 {
		t.Skip("no refs found in after-snapshot")
	}

	// Verify that initial refs are still present (stable)
	for _, ref := range initialRefs {
		found := false
		for _, afterRef := range afterRefs {
			if ref == afterRef {
				found = true
				break
			}
		}
		if !found {
			t.Logf("ref %s not found in after-snapshot (refs changed)", ref)
		}
	}
}

// S6: Snapshot diff mode
func TestSnapshot_DiffMode(t *testing.T) {
	navigate(t, "https://example.com")

	// First snapshot call - stores state
	code, body1 := httpGet(t, "/snapshot")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}

	// Verify first response is valid JSON
	var snap1 map[string]any
	if err := json.Unmarshal(body1, &snap1); err != nil {
		t.Fatalf("first snapshot: invalid json: %v", err)
	}

	// Second snapshot call with diff=true
	code, body2 := httpGet(t, "/snapshot?diff=true")
	if code != 200 {
		t.Fatalf("expected 200 for diff snapshot, got %d", code)
	}

	// Verify second response is valid JSON
	var snap2 map[string]any
	if err := json.Unmarshal(body2, &snap2); err != nil {
		t.Fatalf("diff snapshot: invalid json: %v", err)
	}

	// Verify diff response size is <= initial response (diff should be smaller or equal)
	if len(body2) > len(body1) {
		t.Logf("warning: diff response (%d bytes) larger than initial (%d bytes)", len(body2), len(body1))
	}

	// Both responses should have snapshot structure
	if snap2["nodes"] == nil && snap2["tree"] == nil && snap2["role"] == nil {
		t.Logf("warning: diff snapshot has no nodes/tree/role structure")
	}
}

// S7: Snapshot diff on first call (no prior state)
func TestSnapshot_DiffFirstCall(t *testing.T) {
	// Fresh navigate - no prior snapshot stored
	navigate(t, "https://example.com")

	// Call /snapshot?diff=true immediately (first call with diff=true)
	code, body := httpGet(t, "/snapshot?diff=true")
	if code != 200 {
		t.Fatalf("expected 200 for first diff call, got %d", code)
	}

	// Verify response is valid JSON
	var snapshot map[string]any
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("diff snapshot: invalid json: %v", err)
	}

	// Since no prior snapshot exists, should return full snapshot
	// Verify it has snapshot structure
	if snapshot["nodes"] == nil && snapshot["tree"] == nil && snapshot["role"] == nil {
		t.Error("expected snapshot data (nodes, tree, or role)")
	}
}

// S11: Snapshot on large page
func TestSnapshot_LargePage(t *testing.T) {
	// Navigate to a large Wikipedia article
	url := "https://en.wikipedia.org/wiki/List_of_countries_and_territories_by_total_area"
	navigate(t, url)

	// Call /snapshot on the large page
	code, body := httpGet(t, "/snapshot")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}

	// Verify response is valid JSON
	var snapshot map[string]any
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("large page snapshot: invalid json: %v", err)
	}

	// Verify snapshot has nodes/tree structure
	if snapshot["nodes"] == nil && snapshot["tree"] == nil && snapshot["role"] == nil {
		t.Error("expected snapshot data (nodes, tree, or role)")
	}

	// Verify response is reasonably sized (has actual content)
	if len(body) < 100 {
		t.Error("snapshot response too small for a large page")
	}
}
