package mcp

import (
	"strings"
	"testing"
)

func TestHandleScrape(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scrape", map[string]any{
		"url":     "https://example.com",
		"preview": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/scrape") {
		t.Errorf("expected /scrape path, got %s", text)
	}
	if !strings.Contains(text, `"preview":true`) {
		t.Errorf("preview flag not forwarded: %s", text)
	}
}

func TestHandleScrapeSplitsCommaLists(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scrape", map[string]any{
		"url":  "https://example.com",
		"only": "https://example.com/a, https://example.com/b",
	}, srv)

	// The comma-separated string must reach the server as a JSON array of URLs.
	text := resultText(t, r)
	if !strings.Contains(text, "https://example.com/a") || !strings.Contains(text, "https://example.com/b") {
		t.Errorf("only list not split/forwarded: %s", text)
	}
}

func TestHandleScrapeMissingURL(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scrape", map[string]any{}, srv)
	if !r.IsError {
		t.Error("expected error for missing url")
	}
}
