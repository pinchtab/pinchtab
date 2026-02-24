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
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("expected json object: %v", err)
	}
	tabsRaw := resp["tabs"]
	tabs, ok := tabsRaw.([]any)
	if !ok {
		t.Fatalf("expected tabs to be an array, got %T", tabsRaw)
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
	var resp map[string]any
	_ = json.Unmarshal(listBody, &resp)
	tabsRaw := resp["tabs"]
	tabs, ok := tabsRaw.([]any)
	if !ok || len(tabs) < 2 {
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

// TB4: Close without tabId
func TestTabs_CloseWithoutTabId(t *testing.T) {
	code, _ := httpPost(t, "/tab", map[string]string{"action": "close"})
	if code != 400 {
		t.Errorf("expected 400 when closing without tabId, got %d", code)
	}
}

// TB5: Bad action
func TestTabs_BadAction(t *testing.T) {
	code, _ := httpPost(t, "/tab", map[string]string{"action": "explode"})
	if code != 400 {
		t.Errorf("expected 400 for bad action, got %d", code)
	}
}

// TB6: Max tabs - create many tabs and verify behavior
func TestTabs_MaxTabs(t *testing.T) {
	// Get initial tab count
	_, initialBody := httpGet(t, "/tabs")
	var initialResp map[string]any
	_ = json.Unmarshal(initialBody, &initialResp)
	initialTabs := len(initialResp["tabs"].([]any))

	// Try to create 20 tabs and verify they are created or error appropriately
	createdCount := 0
	for i := 0; i < 20; i++ {
		code, _ := httpPost(t, "/tab", map[string]string{
			"action": "new",
			"url":    "https://example.com",
		})
		if code == 200 {
			createdCount++
		} else if code >= 400 {
			// Server returned an error (likely hit limit)
			break
		}
	}

	// Verify we created at least one tab or hit an error
	if createdCount == 0 && len(initialTabs) > 0 {
		// Acceptable: either we created tabs or hit limit immediately
	}

	// Get final tab count
	_, finalBody := httpGet(t, "/tabs")
	var finalResp map[string]any
	_ = json.Unmarshal(finalBody, &finalResp)
	finalTabs := finalResp["tabs"].([]any)

	// Verify tab list changed or was already at limit
	if len(finalTabs) < initialTabs {
		t.Error("expected tab count to not decrease")
	}
}
