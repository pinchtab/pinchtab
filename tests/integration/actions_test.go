//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

// A1: Click by ref — navigate to example.com, find the "More information..." link, click it
func TestAction_Click(t *testing.T) {
	navigate(t, "https://example.com")
	defer closeCurrentTab(t)

	// Get snapshot to find a clickable ref
	_, snapBody := httpGet(t, "/snapshot?filter=interactive&format=text&tabId="+currentTabID)
	s := string(snapBody)

	// Find a ref like [e0] or [e1] for the link
	ref := findRef(s, "link")
	if ref == "" {
		t.Skip("no clickable link ref found in snapshot")
	}

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "click",
		"ref":   ref,
	})
	if code != 200 {
		t.Errorf("click failed with %d", code)
	}
}

// A4: Press key
func TestAction_Press(t *testing.T) {
	navigate(t, "https://example.com")
	defer closeCurrentTab(t)

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "press",
		"key":   "Escape",
	})
	if code != 200 {
		t.Errorf("press failed with %d", code)
	}
}

// A9: Unknown kind
func TestAction_UnknownKind(t *testing.T) {
	code, _ := httpPost(t, "/action", map[string]string{"kind": "dance"})
	if code != 400 {
		t.Errorf("expected 400 for unknown kind, got %d", code)
	}
}

// A10: Missing kind
func TestAction_MissingKind(t *testing.T) {
	code, _ := httpPost(t, "/action", map[string]string{"ref": "e0"})
	if code != 400 {
		t.Errorf("expected 400 for missing kind, got %d", code)
	}
}

// A11: Ref not found
func TestAction_RefNotFound(t *testing.T) {
	navigate(t, "https://example.com")
	defer closeCurrentTab(t)

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "click",
		"ref":   "e9999",
	})
	// Should be an error (400 or 500)
	if code == 200 {
		t.Error("expected error for non-existent ref")
	}
}

// A12: CSS selector click
func TestAction_CSSSelector(t *testing.T) {
	navigate(t, "https://example.com")
	defer closeCurrentTab(t)

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId":    currentTabID,
		"kind":     "click",
		"selector": "a",
	})
	if code != 200 {
		t.Errorf("CSS selector click failed with %d", code)
	}
}

// Helper: find a ref for a given role in text snapshot
func findRef(snapshot string, role string) string {
	lines := strings.Split(snapshot, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), role) {
			// Look for [eN] pattern
			idx := strings.Index(line, "[e")
			if idx >= 0 {
				end := strings.Index(line[idx:], "]")
				if end > 0 {
					return line[idx+1 : idx+end]
				}
			}
			// Also try eN at start of line
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 1 && trimmed[0] == 'e' {
				parts := strings.Fields(trimmed)
				if len(parts) > 0 && len(parts[0]) <= 5 {
					return parts[0]
				}
			}
		}
	}
	return ""
}

// A2: Type by ref — need an input element
func TestAction_Type(t *testing.T) {
	// Navigate to httpbin form
	navigate(t, "https://httpbin.org/forms/post")
	defer closeCurrentTab(t)

	_, snapBody := httpGet(t, "/snapshot?filter=interactive&format=text&tabId="+currentTabID)
	ref := findRef(string(snapBody), "textbox")
	if ref == "" {
		ref = findRef(string(snapBody), "input")
	}
	if ref == "" {
		t.Skip("no input ref found")
	}

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "type",
		"ref":   ref,
		"text":  "test input",
	})
	if code != 200 {
		t.Errorf("type failed with %d", code)
	}
}

// A3: Fill by ref
func TestAction_Fill(t *testing.T) {
	navigate(t, "https://httpbin.org/forms/post")
	defer closeCurrentTab(t)

	_, snapBody := httpGet(t, "/snapshot?filter=interactive&format=text&tabId="+currentTabID)
	ref := findRef(string(snapBody), "textbox")
	if ref == "" {
		ref = findRef(string(snapBody), "input")
	}
	if ref == "" {
		t.Skip("no input ref found")
	}

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "fill",
		"ref":   ref,
		"text":  "filled value",
	})
	if code != 200 {
		t.Errorf("fill failed with %d", code)
	}
}

// A5: Focus
func TestAction_Focus(t *testing.T) {
	navigate(t, "https://httpbin.org/forms/post")
	defer closeCurrentTab(t)

	_, snapBody := httpGet(t, "/snapshot?filter=interactive&format=text&tabId="+currentTabID)
	ref := findRef(string(snapBody), "textbox")
	if ref == "" {
		t.Skip("no focusable ref found")
	}

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "focus",
		"ref":   ref,
	})
	if code != 200 {
		t.Errorf("focus failed with %d", code)
	}
}

// A8: Scroll
func TestAction_Scroll(t *testing.T) {
	navigate(t, "https://example.com")
	defer closeCurrentTab(t)

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId":     currentTabID,
		"kind":      "scroll",
		"direction": "down",
	})
	if code != 200 {
		t.Errorf("scroll failed with %d", code)
	}
}

// Batch actions (A14)
func TestAction_Batch(t *testing.T) {
	navigate(t, "https://example.com")
	payload := map[string]any{
		"tabId": currentTabID,
		"actions": []map[string]any{
			{"kind": "press", "key": "Escape"},
			{"kind": "scroll", "direction": "down"},
		},
	}
	data, _ := json.Marshal(payload)
	code, body := httpPostRaw(t, "/actions", string(data))
	if code != 200 {
		t.Logf("batch response: %s", body)
		t.Errorf("batch actions failed with %d", code)
	}
}

// A15: Batch empty
func TestAction_BatchEmpty(t *testing.T) {
	code, _ := httpPostRaw(t, "/actions", `{"actions": []}`)
	if code != 400 {
		t.Errorf("expected 400 for empty batch, got %d", code)
	}
}

// A6: Hover
func TestAction_Hover(t *testing.T) {
	navigate(t, "https://example.com")

	// Get snapshot to find a link ref
	_, snapBody := httpGet(t, "/snapshot?filter=interactive&format=text&tabId="+currentTabID)
	s := string(snapBody)

	// Find a link ref
	ref := findRef(s, "link")
	if ref == "" {
		t.Skip("no link ref found in snapshot")
	}

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "hover",
		"ref":   ref,
	})
	if code != 200 {
		t.Errorf("hover failed with %d", code)
	}

	closeCurrentTab(t)
}

// A7: Select
func TestAction_Select(t *testing.T) {
	navigate(t, "https://httpbin.org/forms/post")
	defer closeCurrentTab(t)

	// Get snapshot to find a select element
	_, snapBody := httpGet(t, "/snapshot?filter=interactive&format=text&tabId="+currentTabID)
	ref := findRef(string(snapBody), "combobox")
	if ref == "" {
		ref = findRef(string(snapBody), "select")
	}
	if ref == "" {
		t.Skip("no select ref found in snapshot")
	}

	code, _ := httpPost(t, "/action", map[string]any{
		"tabId": currentTabID,
		"kind":  "select",
		"ref":   ref,
		"value": "opt1",
	})
	if code != 200 {
		t.Errorf("select failed with %d", code)
	}
}

// A13: No tab
func TestAction_NoTab(t *testing.T) {
	navigate(t, "https://example.com")

	code, _ := httpPost(t, "/action", map[string]string{
		"kind":  "click",
		"ref":   "e0",
		"tabId": "nonexistent_xyz",
	})
	// Should be an error (not 200)
	if code == 200 {
		t.Error("expected error for nonexistent tab")
	}
}
