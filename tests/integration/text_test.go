//go:build integration

package integration

import (
	"strings"
	"testing"
)

// T1: Readability mode
func TestText_Readability(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/text")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	if !strings.Contains(s, "Example Domain") {
		t.Error("expected 'Example Domain' in text output")
	}
}

// T2: Raw mode
func TestText_Raw(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/text?mode=raw")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	s := string(body)
	if !strings.Contains(s, "Example Domain") {
		t.Error("expected 'Example Domain' in raw text")
	}
}
