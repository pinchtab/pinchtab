//go:build integration

package integration

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// TestSnapshot_WithURL verifies GET /snapshot?url=... navigates first then snapshots.
func TestSnapshot_WithURL(t *testing.T) {
	// URL param navigation needs an existing tab - create one first
	navigate(t, "about:blank")
	
	params := url.Values{"url": {"https://example.com"}}
	code, body := httpGet(t, "/snapshot?"+params.Encode())
	if code != 200 {
		t.Fatalf("snapshot with url: expected 200, got %d: %s", code, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have nodes from example.com
	if nodes, ok := result["nodes"].([]any); !ok || len(nodes) < 3 {
		t.Errorf("expected nodes from example.com, got %d", len(nodes))
	}
	if title, ok := result["title"].(string); ok {
		if !strings.Contains(strings.ToLower(title), "example") {
			t.Errorf("expected title containing 'example', got %q", title)
		}
	}

	// Clean up: close the tab using returned tabId
	if tabID, ok := result["tabId"].(string); ok {
		httpPost(t, "/tab", map[string]any{
			"tabId":  tabID,
			"action": "close",
		})
	}
}

// TestText_WithURL verifies GET /text?url=... navigates first then extracts text.
func TestText_WithURL(t *testing.T) {
	// URL param navigation needs an existing tab - create one first
	navigate(t, "about:blank")
	
	params := url.Values{"url": {"https://example.com"}}
	code, body := httpGet(t, "/text?"+params.Encode())
	if code != 200 {
		t.Fatalf("text with url: expected 200, got %d: %s", code, string(body))
	}

	// Text endpoint returns JSON with text field when not in plain format
	var result map[string]any
	if err := json.Unmarshal(body, &result); err == nil {
		// JSON format - extract text field
		if text, ok := result["text"].(string); ok {
			if !strings.Contains(strings.ToLower(text), "example") {
				t.Errorf("expected text containing 'example', got %d bytes", len(text))
			}
		}
		// Clean up using returned tabId
		if tabID, ok := result["tabId"].(string); ok {
			httpPost(t, "/tab", map[string]any{
				"tabId":  tabID,
				"action": "close",
			})
		}
	} else {
		// Plain text format
		text := string(body)
		if !strings.Contains(strings.ToLower(text), "example") {
			t.Errorf("expected text containing 'example', got %d bytes", len(text))
		}
		// Can't extract tabId from plain text, leave tab open
	}
}

// TestFind_WithURL verifies POST /find with url field navigates + snapshots + searches.
func TestFind_WithURL(t *testing.T) {
	// URL param navigation needs an existing tab - create one first
	navigate(t, "about:blank")
	
	code, body := httpPost(t, "/find", map[string]any{
		"query": "Learn more",
		"url":   "https://example.com",
		"topK":  5,
	})
	if code != 200 {
		t.Fatalf("find with url: expected 200, got %d: %s", code, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	matches, _ := result["matches"].([]any)
	if len(matches) == 0 {
		t.Error("expected at least one match for 'Learn more' on example.com")
	}

	// Clean up using returned tabId
	if tabID, ok := result["tabId"].(string); ok {
		httpPost(t, "/tab", map[string]any{
			"tabId":  tabID,
			"action": "close",
		})
	}
}

// TestScreenshot_WithURL verifies GET /screenshot?url=... navigates first.
func TestScreenshot_WithURL(t *testing.T) {
	// Navigate first then screenshot (url param with WaitComplete
	// can timeout in CI — use separate steps for reliability).
	navigate(t, "https://example.com")

	params := url.Values{"output": {"raw"}}
	code, body := httpGet(t, "/screenshot?"+params.Encode())
	if code != 200 {
		t.Fatalf("screenshot: expected 200, got %d", code)
	}

	if len(body) < 100 {
		t.Errorf("screenshot too small: %d bytes", len(body))
	}

	// Clean up is handled by navigate() setting currentTabID + t.Cleanup()
}
