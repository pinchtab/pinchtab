package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHandleNavigate(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{
		"url":   "https://example.com",
		"tabId": "t1",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/navigate") {
		t.Errorf("expected /navigate in response, got %s", text)
	}
	if !strings.Contains(text, "https://example.com") {
		t.Errorf("expected URL in response, got %s", text)
	}
}

func TestHandleNavigateMissingURL(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{}, srv)
	if !r.IsError {
		t.Error("expected error for missing URL")
	}
}

func TestHandleNavigateEmptyURL(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{"url": ""}, srv)
	if !r.IsError {
		t.Error("expected error for empty URL")
	}
}

func TestHandleNavigateJavaScript(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{"url": "javascript:void(0)"}, srv)
	if r.IsError {
		t.Errorf("expected javascript: URL to succeed, got error: %s", resultText(t, r))
	}
}

func TestHandleNavigateBareHostname(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{"url": "example.com"}, srv)
	if r.IsError {
		t.Errorf("expected bare hostname to succeed, got error: %s", resultText(t, r))
	}
}

func TestHandleNavigateAnyScheme(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	urls := []string{
		"ftp://files.example.com/readme",
		"chrome://settings",
		"file:///path/to/file.html",
	}
	for _, u := range urls {
		r := callTool(t, "pinchtab_navigate", map[string]any{"url": u}, srv)
		if r.IsError {
			t.Errorf("expected %q to succeed, got error: %s", u, resultText(t, r))
		}
	}
}

func TestHandleNavigate_WithBrowserTarget(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	// When browserTarget is supplied, the forwarded payload includes it.
	r := callTool(t, "pinchtab_navigate", map[string]any{
		"url":           "https://example.com",
		"browserTarget": "chrome",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["browserTarget"].(string); got != "chrome" {
		t.Errorf("browserTarget = %q, want chrome", got)
	}

	// When omitted/empty, the payload must not include the key.
	r2 := callTool(t, "pinchtab_navigate", map[string]any{
		"url":           "https://example.com",
		"browserTarget": "",
	}, srv)

	resp2 := resultJSON(t, r2)
	body2, _ := resp2["body"].(map[string]any)
	if _, ok := body2["browserTarget"]; ok {
		t.Errorf("browserTarget should be omitted when empty, got %v", body2["browserTarget"])
	}
}

func TestHandleNavigateSnapUsesReturnedTabAndBrowserTarget(t *testing.T) {
	var navigateBody map[string]any
	var snapshotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/navigate":
			if err := json.NewDecoder(r.Body).Decode(&navigateBody); err != nil {
				t.Errorf("decode navigate body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tabId": "tab-cloak",
				"url":   "https://example.com",
			})
		case "/snapshot":
			snapshotQuery = r.URL.Query()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"snapshot": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{
		"url":           "https://example.com",
		"browserTarget": "cloak",
		"snap":          true,
	}, srv)
	if r.IsError {
		t.Fatalf("navigate returned error: %s", resultText(t, r))
	}

	if got, _ := navigateBody["browserTarget"].(string); got != "cloak" {
		t.Fatalf("navigate browserTarget = %q, want cloak", got)
	}
	if got := snapshotQuery.Get("tabId"); got != "tab-cloak" {
		t.Fatalf("snapshot tabId = %q, want tab-cloak; query=%v", got, snapshotQuery)
	}
	if got := snapshotQuery.Get("browserTarget"); got != "cloak" {
		t.Fatalf("snapshot browserTarget = %q, want cloak; query=%v", got, snapshotQuery)
	}
}

func TestHandleSnapshot(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"interactive": true,
		"compact":     true,
		"selector":    "#main",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/snapshot") {
		t.Errorf("expected /snapshot path, got %s", text)
	}
}

func TestHandleFrameGet(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_frame", map[string]any{
		"tabId": "t1",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/frame") {
		t.Errorf("expected /frame path, got %s", text)
	}
	resp := resultJSON(t, r)
	if got, _ := resp["method"].(string); got != "GET" {
		t.Errorf("method = %q, want GET", got)
	}
}

func TestHandleFrameSet(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_frame", map[string]any{
		"tabId":  "t1",
		"target": "main",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/frame") {
		t.Errorf("expected /frame path, got %s", text)
	}
	resp := resultJSON(t, r)
	if got, _ := resp["method"].(string); got != "POST" {
		t.Errorf("method = %q, want POST", got)
	}
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["target"].(string); got != "main" {
		t.Errorf("target = %q, want main", got)
	}
}

func TestHandleSnapshotFormatText(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"format": "text",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"format"`) {
		t.Errorf("expected 'format' query param, got %s", text)
	}
	if !strings.Contains(text, "text") {
		t.Errorf("expected format=text in query, got %s", text)
	}
}

func TestHandleSnapshotFormatRejectsUnsupportedValues(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"format": "yaml",
	}, srv)

	if !r.IsError {
		t.Fatal("expected error for unsupported snapshot format")
	}
}

func TestHandleSnapshotNoAnimations(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"noAnimations": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"noAnimations"`) {
		t.Errorf("expected 'noAnimations' query param, got %s", text)
	}
}

func TestHandleScreenshot(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_screenshot", map[string]any{
		"quality":  float64(90),
		"selector": "#hero",
		"css1x":    true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/screenshot") {
		t.Errorf("expected /screenshot, got %s", text)
	}
	if !strings.Contains(text, `"selector"`) {
		t.Errorf("expected selector query param, got %s", text)
	}
	if !strings.Contains(text, `"css1x"`) {
		t.Errorf("expected css1x query param, got %s", text)
	}
}

func TestHandleGetText(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"raw": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/text") {
		t.Errorf("expected /text, got %s", text)
	}
}

func TestHandleGetTextFormat(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"format": "text",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"format"`) {
		t.Errorf("expected 'format' query param, got %s", text)
	}
	if !strings.Contains(text, "text") {
		t.Errorf("expected format=text in query, got %s", text)
	}
}

func TestHandleSnapshotInteractiveSendsFilter(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"interactive": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"filter"`) {
		t.Errorf("expected 'filter' query param, got %s", text)
	}
	if strings.Contains(text, `"interactive"`) && !strings.Contains(text, `"filter"`) {
		t.Error("handler sent ?interactive=true instead of ?filter=interactive")
	}
}

func TestHandleSnapshotCompactSendsFormat(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"compact": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"format"`) {
		t.Errorf("expected 'format' query param, got %s", text)
	}
	if strings.Contains(text, `"compact"`) && !strings.Contains(text, `"format"`) {
		t.Error("handler sent ?compact=true instead of ?format=compact")
	}
}

func TestHandleSnapshotInteractiveCompactCombined(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"interactive": true,
		"compact":     true,
		"selector":    "#main",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"filter"`) {
		t.Errorf("expected 'filter' query param, got %s", text)
	}
	if !strings.Contains(text, `"format"`) {
		t.Errorf("expected 'format' query param, got %s", text)
	}
}

func TestHandleSnapshotMaxTokens(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"interactive": true,
		"maxTokens":   float64(300),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"maxTokens"`) {
		t.Errorf("expected 'maxTokens' query param, got %s", text)
	}
	if !strings.Contains(text, "300") {
		t.Errorf("expected maxTokens=300 in query, got %s", text)
	}
}

func TestHandleSnapshotDepth(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"depth": float64(3),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"depth"`) {
		t.Errorf("expected 'depth' query param, got %s", text)
	}
}

func TestHandleSnapshotMaxTokensZeroIgnored(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_snapshot", map[string]any{
		"maxTokens": float64(0),
	}, srv)

	text := resultText(t, r)
	if strings.Contains(text, `"maxTokens"`) {
		t.Errorf("maxTokens=0 should not be sent, got %s", text)
	}
}

func TestHandleGetTextMaxChars(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"maxChars": float64(3000),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"maxChars"`) {
		t.Errorf("expected 'maxChars' query param, got %s", text)
	}
	if !strings.Contains(text, "3000") {
		t.Errorf("expected maxChars=3000 in query, got %s", text)
	}
}

func TestHandleGetTextMaxCharsZeroIgnored(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"raw": true,
	}, srv)

	text := resultText(t, r)
	if strings.Contains(text, `"maxChars"`) {
		t.Errorf("maxChars should not be sent when not specified, got %s", text)
	}
}
