//go:build integration

package integration

import (
	"testing"
)

// SS1: Basic screenshot
func TestScreenshot_Basic(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/screenshot")
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	// Default returns JSON with base64
	if len(body) < 100 {
		t.Error("screenshot response too small")
	}
}

// SS2: Raw screenshot
// Note: May skip in CI if headless Chrome has display limitations.
// See tests/manual/screenshot-raw.md for headed Chrome testing.
func TestScreenshot_Raw(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpGet(t, "/screenshot?raw=true")
	if code != 200 {
		t.Skipf("screenshot raw returned %d (headless display limitation), skipping", code)
	}
	// JPEG starts with FF D8
	if len(body) < 2 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Error("expected raw JPEG data (FF D8 header)")
	}
}
