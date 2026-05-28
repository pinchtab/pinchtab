package mcp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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
		"quality":        float64(90),
		"selector":       "#hero",
		"css1x":          true,
		"beyondViewport": true,
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
	if !strings.Contains(text, `"beyondViewport"`) {
		t.Errorf("expected beyondViewport query param, got %s", text)
	}
}

func TestHandleScreenshotEnvelopeReturnsImage(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("PNGBYTES"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"format": "png",
			"base64": encoded,
		})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_screenshot", map[string]any{"format": "png"}, srv)

	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, r))
	}
	if len(r.Content) < 2 {
		t.Fatalf("expected text+image content, got %d blocks", len(r.Content))
	}
	var env struct {
		Format      string           `json:"format"`
		Annotations []map[string]any `json:"annotations"`
	}
	if err := json.Unmarshal([]byte(resultText(t, r)), &env); err != nil {
		t.Fatalf("text block is not JSON: %v", err)
	}
	if env.Format != "png" {
		t.Errorf("format = %q, want png", env.Format)
	}
	if env.Annotations == nil || len(env.Annotations) != 0 {
		t.Errorf("annotations = %#v, want empty array", env.Annotations)
	}
	img, ok := r.Content[1].(mcp.ImageContent)
	if !ok {
		t.Fatalf("content[1] = %T, want ImageContent", r.Content[1])
	}
	if img.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want image/png", img.MIMEType)
	}
	if img.Data != encoded {
		t.Errorf("image data mismatch: got %q want %q", img.Data, encoded)
	}
}

func TestHandleScreenshotAnnotateCarriesAnnotations(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("JPEGBYTES"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("annotate") != "true" {
			t.Errorf("expected annotate=true, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"format": "jpeg",
			"base64": encoded,
			"annotations": []map[string]any{
				{
					"ref":  "e5",
					"role": "button",
					"name": "Submit",
					"tag":  "button",
					"box":  map[string]float64{"x": 10, "y": 20, "w": 30, "h": 40},
				},
			},
		})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_screenshot", map[string]any{"annotate": true}, srv)

	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, r))
	}
	text := resultText(t, r)
	if !strings.Contains(text, `"annotations"`) || !strings.Contains(text, `"e5"`) {
		t.Errorf("expected annotations JSON in text block, got %q", text)
	}
	img, ok := r.Content[1].(mcp.ImageContent)
	if !ok {
		t.Fatalf("content[1] = %T, want ImageContent", r.Content[1])
	}
	if img.MIMEType != "image/jpeg" {
		t.Errorf("MIMEType = %q, want image/jpeg", img.MIMEType)
	}
	if img.Data != encoded {
		t.Errorf("image data mismatch")
	}
}

func TestHandleScreenshotHTTPErrorPassesThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"no tab"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_screenshot", nil, srv)

	if !r.IsError {
		t.Fatalf("expected error result for 404, got %s", resultText(t, r))
	}
	if !strings.Contains(resultText(t, r), "no tab") {
		t.Errorf("expected upstream error body in message, got %q", resultText(t, r))
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
