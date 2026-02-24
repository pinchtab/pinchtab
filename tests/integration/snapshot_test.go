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
