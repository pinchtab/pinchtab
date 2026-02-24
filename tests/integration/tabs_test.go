//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// TB1: List tabs
func TestTabs_List(t *testing.T) {
	code, body := httpGet(t, "/tabs")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	var tabs []any
	if err := json.Unmarshal(body, &tabs); err != nil {
		t.Fatalf("expected json array: %v", err)
	}
	if len(tabs) == 0 {
		t.Error("expected at least one tab")
	}
}

// TB2: New tab
func TestTabs_New(t *testing.T) {
	code, body := httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "https://example.com",
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}

	// Verify tab count increased
	_, listBody := httpGet(t, "/tabs")
	var tabs []any
	_ = json.Unmarshal(listBody, &tabs)
	if len(tabs) < 2 {
		t.Error("expected at least 2 tabs after creating new tab")
	}
}

// TB3: Close tab
func TestTabs_Close(t *testing.T) {
	// Create a tab to close
	_, newBody := httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "https://example.com",
	})
	var newTab map[string]any
	_ = json.Unmarshal(newBody, &newTab)
	tabID, _ := newTab["tabId"].(string)
	if tabID == "" {
		t.Skip("no tabId returned from new tab")
	}

	code, _ := httpPost(t, "/tab", map[string]string{
		"action": "close",
		"tabId":  tabID,
	})
	if code != 200 {
		t.Errorf("close tab failed with %d", code)
	}
}

// TB5: Bad action
func TestTabs_BadAction(t *testing.T) {
	code, _ := httpPost(t, "/tab", map[string]string{"action": "explode"})
	if code != 400 {
		t.Errorf("expected 400 for bad action, got %d", code)
	}
}
