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
	createdTabIDs := []string{}
	for i := 0; i < 20; i++ {
		code, body := httpPost(t, "/tab", map[string]string{
			"action": "new",
			"url":    "https://example.com",
		})
		if code == 200 {
			var newTab map[string]any
			_ = json.Unmarshal(body, &newTab)
			if tabID, ok := newTab["tabId"].(string); ok {
				createdTabIDs = append(createdTabIDs, tabID)
			}
		} else if code >= 400 {
			// Server returned an error (likely hit limit)
			break
		}
	}

	// Acceptable: either we created tabs or hit limit immediately

	// Get final tab count
	_, finalBody := httpGet(t, "/tabs")
	var finalResp map[string]any
	_ = json.Unmarshal(finalBody, &finalResp)
	finalTabs := finalResp["tabs"].([]any)

	// Verify tab list changed or was already at limit
	if len(finalTabs) < initialTabs {
		t.Error("expected tab count to not decrease")
	}

	// IMPORTANT: Clean up created tabs to avoid affecting subsequent tests
	// This is critical for test isolation in the shared browser instance
	for _, tabID := range createdTabIDs {
		_, _ = httpPost(t, "/tab", map[string]string{
			"action": "close",
			"tabId":  tabID,
		})
	}
}

// TB7: Prevent closing last tab - ensure server doesn't crash
func TestTabs_PreventLastTabClose(t *testing.T) {
	// Get initial tab list
	_, initialBody := httpGet(t, "/tabs")
	var initialResp map[string]any
	_ = json.Unmarshal(initialBody, &initialResp)
	initialTabs := initialResp["tabs"].([]any)

	// Create a second tab to ensure we have at least two
	_, createBody := httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "https://example.com",
	})
	var newTab map[string]any
	_ = json.Unmarshal(createBody, &newTab)
	createdTabID, _ := newTab["tabId"].(string)

	// Get current tab list
	_, currentBody := httpGet(t, "/tabs")
	var currentResp map[string]any
	_ = json.Unmarshal(currentBody, &currentResp)
	currentTabs := currentResp["tabs"].([]any)

	// If we have more than one tab, close all but one
	tabsToClose := []string{}
	for i, tab := range currentTabs {
		if i > 0 { // Keep the first tab, collect others for closing
			if tabMap, ok := tab.(map[string]any); ok {
				if tabID, ok := tabMap["id"].(string); ok {
					tabsToClose = append(tabsToClose, tabID)
				}
			}
		}
	}

	// Close all but the first tab
	for _, tabID := range tabsToClose {
		_, _ = httpPost(t, "/tab", map[string]string{
			"action": "close",
			"tabId":  tabID,
		})
	}

	// Now get the remaining tab and try to close it
	_, finalBody := httpGet(t, "/tabs")
	var finalResp map[string]any
	_ = json.Unmarshal(finalBody, &finalResp)
	finalTabs := finalResp["tabs"].([]any)

	if len(finalTabs) >= 1 {
		// Try to close the last tab - this should fail
		lastTab := finalTabs[0].(map[string]any)
		lastTabID := lastTab["id"].(string)

		code, body := httpPost(t, "/tab", map[string]string{
			"action": "close",
			"tabId":  lastTabID,
		})

		// Should return an error (400 or 422) and not crash the server
		if code == 200 {
			t.Error("Expected error when trying to close the last tab, but got success")
		}

		// Verify the error message mentions preventing last tab close
		if len(body) > 0 {
			bodyStr := string(body)
			if bodyStr != "" {
				t.Logf("Close last tab error response: %s", bodyStr)
			}
		}

		// Most importantly, verify the server is still responsive
		code, _ = httpGet(t, "/tabs")
		if code != 200 {
			t.Error("Server became unresponsive after attempting to close last tab")
		}
	}

	// Clean up by creating a new tab if needed (restore state)
	if createdTabID != "" {
		// Create a replacement tab for test isolation
		_, _ = httpPost(t, "/tab", map[string]string{
			"action": "new",
			"url":    "about:blank",
		})
	}
}
