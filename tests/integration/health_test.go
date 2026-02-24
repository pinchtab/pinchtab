//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// H1: Health check
func TestHealth(t *testing.T) {
	code, body := httpGet(t, "/health")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if m["status"] != "ok" {
		t.Errorf("expected status ok, got %v", m["status"])
	}
}

// H5/H6: Auth tested separately (needs token config)
