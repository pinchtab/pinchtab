//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// C1: Get cookies
func TestCookies_Get(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/cookies?tabId="+currentTabID)
	if code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// Should have cookies array
	cookiesRaw := resp["cookies"]
	cookies, ok := cookiesRaw.([]any)
	if !ok {
		t.Fatalf("expected cookies to be an array, got %T", cookiesRaw)
	}

	// Verify structure of cookies if any exist
	for _, c := range cookies {
		cookie, ok := c.(map[string]any)
		if !ok {
			t.Errorf("expected cookie to be an object, got %T", c)
			continue
		}

		// Check required fields
		if cookie["name"] == nil {
			t.Error("expected 'name' field in cookie")
		}
		if cookie["value"] == nil {
			t.Error("expected 'value' field in cookie")
		}
		if cookie["domain"] == nil {
			t.Error("expected 'domain' field in cookie")
		}
		if cookie["path"] == nil {
			t.Error("expected 'path' field in cookie")
		}
	}
}

// C2: Set cookies
func TestCookies_Set(t *testing.T) {
	navigate(t, "https://example.com")

	// Set a cookie
	code, body := httpPost(t, "/cookies", map[string]any{
		"tabId": currentTabID,
		"url":   "https://example.com",
		"cookies": []map[string]any{
			{
				"name":  "test_cookie",
				"value": "test_value",
				"path":  "/",
			},
		},
	})

	if code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// Check response indicates success
	setCount, ok := resp["set"].(float64)
	if !ok || setCount != 1 {
		t.Errorf("expected 'set' field = 1, got %v", resp["set"])
	}

	// Verify the cookie was set by getting cookies again
	code2, body2 := httpGet(t, "/cookies?url=https://example.com&tabId="+currentTabID)
	if code2 != 200 {
		t.Fatalf("GET /cookies failed: %d", code2)
	}

	var getRespBody map[string]any
	if err := json.Unmarshal(body2, &getRespBody); err != nil {
		t.Fatalf("invalid json from GET: %v", err)
	}

	cookiesRaw := getRespBody["cookies"]
	cookies, ok := cookiesRaw.([]any)
	if !ok {
		t.Errorf("expected cookies array, got %T", cookiesRaw)
	}

	// Look for our cookie
	found := false
	for _, c := range cookies {
		cookie, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cookie["name"] == "test_cookie" && cookie["value"] == "test_value" {
			found = true
			break
		}
	}

	if !found {
		t.Error("set cookie not found in subsequent GET /cookies")
	}
}

// C3: Get cookies no tab
func TestCookies_GetNoTab(t *testing.T) {
	// Try to get cookies from a non-existent tab
	code, body := httpGet(t, "/cookies?tabId=nonexistent_tab_12345")
	if code == 200 {
		t.Errorf("expected error when getting cookies from non-existent tab, got 200 (body: %s)", body)
	}
}

// C4: Set cookies bad JSON
func TestCookies_SetBadJSON(t *testing.T) {
	navigate(t, "https://example.com")

	code, body := httpPostRaw(t, "/cookies", "{broken")
	if code != 400 {
		t.Errorf("expected 400 for bad JSON, got %d (body: %s)", code, body)
	}
}

// C5: Set cookies empty
func TestCookies_SetEmpty(t *testing.T) {
	navigate(t, "https://example.com")

	// Post with empty cookies array
	code, body := httpPost(t, "/cookies", map[string]any{
		"tabId":   currentTabID,
		"url":     "https://example.com",
		"cookies": []map[string]any{},
	})

	// The handler returns 400 for empty cookies array
	if code != 400 {
		t.Errorf("expected 400 for empty cookies array, got %d (body: %s)", code, body)
	}
}
