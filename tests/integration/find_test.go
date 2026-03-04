//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

func TestFind_BasicMatch(t *testing.T) {
	navigate(t, "https://example.com")

	// /find requires a cached snapshot — take one first.
	sCode, _ := httpGet(t, snapshotPath("/snapshot"))
	if sCode != 200 {
		t.Fatalf("snapshot prerequisite failed: %d", sCode)
	}

	// Don't pass tabId — shorthand /find uses the active tab.
	code, body := httpPost(t, "/find", map[string]any{
		"query": "Learn more",
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d: %s", code, string(body))
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	matches, ok := resp["matches"].([]any)
	if !ok {
		t.Fatalf("expected matches array, got: %v", resp)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match for 'Learn more' on example.com")
	}

	// First match should have ref, score, and element info.
	first, ok := matches[0].(map[string]any)
	if !ok {
		t.Fatalf("expected match object, got: %v", matches[0])
	}
	if first["ref"] == nil {
		t.Error("match missing 'ref' field")
	}
	if first["score"] == nil {
		t.Error("match missing 'score' field")
	}

	// Response should include metadata.
	if resp["best_ref"] == nil {
		t.Error("response missing 'best_ref' field")
	}
	if resp["strategy"] == nil {
		t.Error("response missing 'strategy' field")
	}
}

func TestFind_NoMatch(t *testing.T) {
	navigate(t, "https://example.com")

	// /find requires a cached snapshot.
	sCode, _ := httpGet(t, snapshotPath("/snapshot"))
	if sCode != 200 {
		t.Fatalf("snapshot prerequisite failed: %d", sCode)
	}

	code, body := httpPost(t, "/find", map[string]any{
		"query": "xyzzy_nonexistent_element_12345",
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d: %s", code, string(body))
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	matches, ok := resp["matches"].([]any)
	if !ok {
		t.Fatalf("expected matches array, got: %v", resp)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for nonsense query, got %d", len(matches))
	}
}

func TestFind_MissingQuery(t *testing.T) {
	navigate(t, "https://example.com")

	code, _ := httpPost(t, "/find", map[string]any{})
	if code != 400 {
		t.Fatalf("expected 400 for missing query, got %d", code)
	}
}

func TestFind_InvalidTab(t *testing.T) {
	// Passing a nonexistent tabId should fail.
	code, _ := httpPost(t, "/find", map[string]any{
		"query": "anything",
		"tabId": "tab_nonexistent_12345",
	})
	if code == 200 {
		t.Error("expected error for invalid tab, got 200")
	}
}
