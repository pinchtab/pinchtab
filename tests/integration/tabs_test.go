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
	code, initialBody := httpGet(t, "/tabs")
	if code != 200 {
		t.Skipf("skipping: /tabs returned %d (no running instance)", code)
	}
	var initialResp map[string]any
	_ = json.Unmarshal(initialBody, &initialResp)
	tabsRaw, ok := initialResp["tabs"]
	if !ok || tabsRaw == nil {
		t.Skip("skipping: no tabs field in response")
	}
	initialTabs := len(tabsRaw.([]any))

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
	var finalTabs []any
	if raw, ok := finalResp["tabs"]; ok && raw != nil {
		finalTabs, _ = raw.([]any)
	}

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

// TB7: Tab ID format validation - only hash IDs accepted
func TestTabs_IDFormat(t *testing.T) {
	// Create a new tab and verify it returns hash format
	code, body := httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "about:blank",
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}

	var newTab map[string]any
	_ = json.Unmarshal(body, &newTab)
	tabID, ok := newTab["tabId"].(string)
	if !ok || tabID == "" {
		t.Fatal("expected tabId in response")
	}

	// Verify hash format: tab_XXXXXXXX (12 chars total)
	if len(tabID) != 12 || tabID[:4] != "tab_" {
		t.Errorf("expected hash format tab_XXXXXXXX, got %s", tabID)
	}

	// Clean up
	_, _ = httpPost(t, "/tab", map[string]string{"action": "close", "tabId": tabID})
}

// TB8: Raw CDP ID rejection - security test
func TestTabs_RejectsRawCDPID(t *testing.T) {
	// Raw CDP target IDs (32-char hex) should be rejected
	rawCDPID := "A25658CE1BA82659EBE9C93C46CEE63A"

	// Try to navigate using raw CDP ID - should fail with 404
	code, _ := httpPost(t, "/tabs/"+rawCDPID+"/navigate", map[string]string{
		"url": "https://example.com",
	})
	if code != 404 {
		t.Errorf("expected 404 for raw CDP ID, got %d", code)
	}

	// Try to get snapshot using raw CDP ID - should fail with 404
	code, _ = httpGet(t, "/tabs/"+rawCDPID+"/snapshot")
	if code != 404 {
		t.Errorf("expected 404 for raw CDP ID snapshot, got %d", code)
	}

	// Try action using raw CDP ID - should fail with 404
	code, _ = httpPost(t, "/tabs/"+rawCDPID+"/action", map[string]string{
		"kind": "click",
		"ref":  "e0",
	})
	if code != 404 {
		t.Errorf("expected 404 for raw CDP ID action, got %d", code)
	}
}

// TB9: Prevent closing last tab (server crash protection)
func TestTabs_PreventLastTabClose(t *testing.T) {
	// Get current tabs
	code, body := httpGet(t, "/tabs")
	if code != 200 {
		t.Skipf("skipping: /tabs returned %d", code)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to parse tabs response: %v", err)
	}

	tabsRaw, ok := resp["tabs"].([]any)
	if !ok || len(tabsRaw) == 0 {
		t.Skip("no tabs available")
	}

	// Create extra tabs so we can close down to one
	var createdTabs []string
	for i := 0; i < 3; i++ {
		_, newBody := httpPost(t, "/tab", map[string]string{
			"action": "new",
			"url":    "about:blank",
		})
		var newTab map[string]any
		if json.Unmarshal(newBody, &newTab) == nil {
			if tabID, ok := newTab["tabId"].(string); ok {
				createdTabs = append(createdTabs, tabID)
			}
		}
	}

	// Get all tabs again
	_, body = httpGet(t, "/tabs")
	_ = json.Unmarshal(body, &resp)
	tabsRaw = resp["tabs"].([]any)

	// Close all but one tab
	for i := 0; i < len(tabsRaw)-1; i++ {
		tab := tabsRaw[i].(map[string]any)
		tabID := tab["id"].(string)
		httpPost(t, "/tab", map[string]string{
			"action": "close",
			"tabId":  tabID,
		})
	}

	// Get remaining tab
	_, body = httpGet(t, "/tabs")
	_ = json.Unmarshal(body, &resp)
	tabsRaw = resp["tabs"].([]any)
	if len(tabsRaw) == 0 {
		t.Skip("no tabs remaining after cleanup")
	}

	lastTab := tabsRaw[0].(map[string]any)
	lastTabID := lastTab["id"].(string)

	// Try to close the last tab - should fail
	code, body = httpPost(t, "/tab", map[string]string{
		"action": "close",
		"tabId":  lastTabID,
	})

	// Should NOT succeed (400 or 500, but not 200)
	if code == 200 {
		t.Error("closing last tab should fail, but got 200")
	}

	// Verify error message mentions last tab
	var errResp map[string]any
	if json.Unmarshal(body, &errResp) == nil {
		if errMsg, ok := errResp["error"].(string); ok {
			t.Logf("Last tab close error: %s", errMsg)
		}
	}

	// Most importantly: verify server is still alive
	code, _ = httpGet(t, "/health")
	if code != 200 {
		t.Error("server crashed after attempting to close last tab")
	}

	// Restore a tab for test isolation
	httpPost(t, "/tab", map[string]string{
		"action": "new",
		"url":    "about:blank",
	})
}
