package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestHandleNavigateCarriesTheDocumentStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tabId": "t1", "url": "https://example.com/missing", "status": 404})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_navigate", map[string]any{"url": "https://example.com/missing"}, srv)
	if r.IsError {
		t.Fatalf("an HTTP error page is a completed navigation, got error: %s", resultText(t, r))
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(resultText(t, r)), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != float64(404) {
		t.Fatalf("status = %v, want 404: %s", result["status"], resultText(t, r))
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

func TestHandleNavigateSnapUsesReturnedTab(t *testing.T) {
	var snapshotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/navigate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tabId": "tab-new",
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
		"url":  "https://example.com",
		"snap": true,
	}, srv)
	if r.IsError {
		t.Fatalf("navigate returned error: %s", resultText(t, r))
	}

	if got := snapshotQuery.Get("tabId"); got != "tab-new" {
		t.Fatalf("snapshot tabId = %q, want tab-new; query=%v", got, snapshotQuery)
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
		"scale":          float64(0.5),
		"beyondViewport": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/screenshot") {
		t.Errorf("expected /screenshot, got %s", text)
	}
	if !strings.Contains(text, `"selector"`) {
		t.Errorf("expected selector query param, got %s", text)
	}
	if !strings.Contains(text, `"scale"`) {
		t.Errorf("expected scale query param, got %s", text)
	}
	if !strings.Contains(text, `"beyondViewport"`) {
		t.Errorf("expected beyondViewport query param, got %s", text)
	}
	// Inline output is requested explicitly, not left to the server default.
	if !strings.Contains(text, `"output"`) || !strings.Contains(text, "inline") {
		t.Errorf("expected output=inline query param, got %s", text)
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

func TestHandleGetTextModeMarkdown(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"mode": "markdown",
	}, srv)

	if got := getTextQueryValue(t, r, "mode"); got != "markdown" {
		t.Errorf("outbound query mode = %q, want markdown", got)
	}
}

func TestHandleGetTextRawMapsToModeRaw(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"raw": true,
	}, srv)

	if got := getTextQueryValue(t, r, "mode"); got != "raw" {
		t.Errorf("outbound query mode = %q, want raw for raw:true", got)
	}
}

func TestHandleGetTextModeSupersedesRaw(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_get_text", map[string]any{
		"mode": "markdown",
		"raw":  true,
	}, srv)

	if got := getTextQueryValue(t, r, "mode"); got != "markdown" {
		t.Errorf("outbound query mode = %q, want markdown; mode must win over raw when both are set", got)
	}
}

// The outbound-query tests above prove pinchtab_get_text SENDS mode=markdown;
// this proves it RETURNS the Markdown the server produces — the coverage the
// query tests cannot give, since they never read a response body. It drives the
// tool against a server that answers /text with Markdown structures when (and only
// when) mode=markdown, and asserts a heading, link, list and table survive into
// the tool result. The default read gets plain text, so the mode is what changes
// the shape rather than the fixture.
func TestHandleGetTextModeMarkdownReturnsMarkdownBody(t *testing.T) {
	const markdownBody = "# Markdown Fixture\n\nA paragraph with an [inline link](https://example.com/link).\n\n- one\n- two\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	const plainBody = "Markdown Fixture A paragraph with an inline link one two A B 1 2"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"url": "http://fixtures/markdown.html", "title": "Markdown Fixture"}
		if r.URL.Query().Get("mode") == "markdown" {
			body["extraction"] = "markdown"
			body["text"] = markdownBody
		} else {
			body["extraction"] = "readability"
			body["text"] = plainBody
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	got := resultJSON(t, callTool(t, "pinchtab_get_text", map[string]any{"mode": "markdown"}, srv))
	if got["extraction"] != "markdown" {
		t.Errorf("extraction = %v, want markdown", got["extraction"])
	}
	text, _ := got["text"].(string)
	for _, want := range []string{"# ", "](", "- ", "|"} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown tool result lost the %q structure:\n%s", want, text)
		}
	}

	// The same tool without mode returns plain text, so the Markdown above is the
	// mode's doing and not the server answering markdown to everything.
	plain := resultJSON(t, callTool(t, "pinchtab_get_text", map[string]any{}, srv))
	if pt, _ := plain["text"].(string); strings.Contains(pt, "](") {
		t.Errorf("default get_text should not carry Markdown link syntax:\n%s", pt)
	}
}

// getTextQueryValue reads the first value the mock recorded for a query key. The
// mock echoes r.URL.Query() (a map of string→[]string) into resp["query"].
func getTextQueryValue(t *testing.T, r *mcp.CallToolResult, key string) string {
	t.Helper()
	query, ok := resultJSON(t, r)["query"].(map[string]any)
	if !ok {
		t.Fatalf("result carried no query object: %s", resultText(t, r))
	}
	values, ok := query[key].([]any)
	if !ok || len(values) == 0 {
		return ""
	}
	s, _ := values[0].(string)
	return s
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

func TestHandleCaptureForwardsBrowser(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("JPEGBYTES"))
	var captureQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captureQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"image": map[string]any{"format": "jpeg", "base64": encoded},
		})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_capture", map[string]any{"browser": "cloak"}, srv)
	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, r))
	}
	if got := captureQuery.Get("browser"); got != "cloak" {
		t.Fatalf("capture browser = %q, want cloak; query=%v", got, captureQuery)
	}
}

// MCP clients and LLM-generated tool calls routinely stringify numbers. Every
// other numeric argument accepts that through optInt; the response-budget
// arguments must too, or the cap is silently dropped and the caller gets a full
// page instead of an error.
func TestNumericArgumentsAcceptStringForm(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		key   string
		value string
	}{
		{"pinchtab_get_text", "maxChars", "3000"},
		{"pinchtab_snapshot", "maxTokens", "1500"},
		{"pinchtab_snapshot", "depth", "4"},
		{"pinchtab_capture", "depth", "4"},
		// Fractional arguments went through the other accessor, which stayed
		// strict after the budget controls were moved off it.
		{"pinchtab_screenshot", "scale", "0.5"},
		{"pinchtab_screenshot", "quality", "60"},
		{"pinchtab_capture", "scale", "0.5"},
		{"pinchtab_capture", "quality", "60"},
		{"pinchtab_network", "limit", "25"},
		{"pinchtab_network", "bufferSize", "500"},
	} {
		t.Run(tc.tool+"/"+tc.key, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()

			text := resultText(t, callTool(t, tc.tool, map[string]any{tc.key: tc.value}, srv))
			if !strings.Contains(text, `"`+tc.key+`"`) {
				t.Errorf("%s=%q was dropped from the query: %s", tc.key, tc.value, text)
			}
			if !strings.Contains(text, tc.value) {
				t.Errorf("%s=%q did not reach the query: %s", tc.key, tc.value, text)
			}
		})
	}
}

// Coordinates take a different route to a different place — resolveXY, into a
// POST body rather than a query — so the query-based table above cannot speak
// for them. Both must parse or hasXY goes false and the click falls back to a
// selector that was never given.
func TestCoordinateArgumentsAcceptStringForm(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	text := resultText(t, callTool(t, "pinchtab_click", map[string]any{"x": "10", "y": "20"}, srv))
	for _, want := range []string{`"hasXY":true`, `"x":10`, `"y":20`} {
		if !strings.Contains(text, want) {
			t.Errorf("string-form coordinates: want %s in %s", want, text)
		}
	}
}

// A dropped delta does not merely lose a number, it changes the interaction:
// hasDeltaY gates the mouse-wheel branch, so without it a scroll degrades to a
// scrollY jump or lets `direction` synthesise a magnitude the caller never
// asked for. Deltas are also the only sign-bearing numeric arguments, and the
// only ones no positivity guard protects.
func TestScrollDeltaAcceptsStringForm(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	text := resultText(t, callTool(t, "pinchtab_scroll", map[string]any{"deltaY": "-300"}, srv))
	for _, want := range []string{`"kind":"mouse-wheel"`, `"deltaY":-300`} {
		if !strings.Contains(text, want) {
			t.Errorf("string-form deltaY: want %s in %s", want, text)
		}
	}
}

// The wait timeout is the third route a numeric argument can take: a POST body
// built by callWaitEndpoint, truncated through int() on the way. Neither the
// query table nor the coordinate test passes through it.
func TestWaitTimeoutAcceptsStringForm(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	text := resultText(t, callTool(t, "pinchtab_wait",
		map[string]any{"for": "selector", "value": "#done", "timeout": "5"}, srv))
	if !strings.Contains(text, `"timeout":5`) {
		t.Errorf("string-form timeout did not reach the wait body: %s", text)
	}
}

// The sibling of TestNumericArgumentsAcceptStringForm. Booleans are the larger
// surface — twenty call sites — and withBounds is the only opt-out among them,
// so dropping its string form means an explicit "off" is ignored and the
// response looks like the default rather than like the request.
func TestBooleanArgumentsAcceptStringForm(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		args  map[string]any
		want  string
		label string
	}{
		{"pinchtab_get_text", map[string]any{"raw": "true"}, "mode", "raw=true sets mode=raw"},
		{"pinchtab_capture", map[string]any{"beyondViewport": "true"}, "beyondViewport", "opt-in flag applies"},
		{"pinchtab_capture", map[string]any{"withBounds": "false"}, `"withBounds":["false"]`, "explicit off is forwarded"},
		{"pinchtab_scrape", map[string]any{"url": "https://example.test/", "noBrowser": "TRUE"}, "noBrowser", "ParseBool spellings are accepted"},
	} {
		t.Run(tc.tool+"/"+tc.label, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()

			text := resultText(t, callTool(t, tc.tool, tc.args, srv))
			if !strings.Contains(text, tc.want) {
				t.Errorf("%s: %q missing from the outbound request: %s", tc.label, tc.want, text)
			}
		})
	}
}

// historyRecorder captures what actually went on the wire, because the two things
// these tools must get right are both invisible in a response: /back, /forward and
// /reload never parse a request body, so a body is not merely redundant but a sign
// the handler was copied from pinchtab_navigate, and browser has to travel in the
// query or the router cannot see it.
// Distinct from routing_test.go's recordingPinchTab, which keeps only the last
// request and only its parsed JSON body: snap makes two calls, and "was any body
// sent at all" is the assertion these tools need, so the raw bytes and the
// Content-Type both have to survive.
type historyRequest struct {
	method      string
	path        string
	query       url.Values
	body        string
	contentType string
}

func historyRecorder(t *testing.T) (*httptest.Server, *[]historyRequest) {
	t.Helper()
	seen := &[]historyRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*seen = append(*seen, historyRequest{
			method:      r.Method,
			path:        r.URL.Path,
			query:       r.URL.Query(),
			body:        string(raw),
			contentType: r.Header.Get("Content-Type"),
		})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/snapshot" {
			_, _ = w.Write([]byte(`{"nodes":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"tabId":"ABC123","url":"https://example.com/landed"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func TestHistoryToolsPostToTheirRouteWithNoBody(t *testing.T) {
	for tool, verb := range map[string]string{
		"pinchtab_back":    "back",
		"pinchtab_forward": "forward",
		"pinchtab_reload":  "reload",
	} {
		t.Run(tool, func(t *testing.T) {
			srv, seen := historyRecorder(t)

			result := callTool(t, tool, map[string]any{}, srv)
			if result.IsError {
				t.Fatalf("%s failed: %s", tool, resultText(t, result))
			}
			if len(*seen) != 1 {
				t.Fatalf("requests = %d, want exactly one", len(*seen))
			}
			got := (*seen)[0]

			if got.method != http.MethodPost {
				t.Errorf("method = %s, want POST", got.method)
			}
			if got.path != "/"+verb {
				t.Errorf("path = %q, want /%s", got.path, verb)
			}
			if got.body != "" {
				t.Errorf("a body was sent to /%s, which never parses one: %q", verb, got.body)
			}
			if got.contentType != "" {
				t.Errorf("Content-Type = %q on a bodyless request", got.contentType)
			}

			// The landed URL and the tab ID both come back from the response.
			text := resultText(t, result)
			for _, want := range []string{"ABC123", "https://example.com/landed"} {
				if !strings.Contains(text, want) {
					t.Errorf("result %q does not carry %q", text, want)
				}
			}
		})
	}
}

func TestHistoryToolsScopeToTheTabPathWhenGivenATabID(t *testing.T) {
	for tool, verb := range map[string]string{
		"pinchtab_back":    "back",
		"pinchtab_forward": "forward",
		"pinchtab_reload":  "reload",
	} {
		t.Run(tool, func(t *testing.T) {
			srv, seen := historyRecorder(t)

			callTool(t, tool, map[string]any{"tabId": "t1"}, srv)
			if len(*seen) != 1 {
				t.Fatalf("requests = %d, want exactly one", len(*seen))
			}
			got := (*seen)[0]
			if want := "/tabs/t1/" + verb; got.path != want {
				t.Errorf("path = %q, want %q", got.path, want)
			}
			if got.body != "" {
				t.Errorf("a body was sent alongside the tab-scoped path: %q", got.body)
			}
		})
	}
}

// browser decides WHICH instance serves the call and that router reads the query
// only, so a body value — the form pinchtab_navigate uses — would be invisible here.
func TestHistoryToolsForwardBrowserInTheQuery(t *testing.T) {
	for _, tool := range []string{"pinchtab_back", "pinchtab_forward", "pinchtab_reload"} {
		t.Run(tool, func(t *testing.T) {
			srv, seen := historyRecorder(t)

			callTool(t, tool, map[string]any{"browser": "cloak"}, srv)
			got := (*seen)[0]

			if got.query.Get("browser") != "cloak" {
				t.Errorf("query browser = %q, want cloak — the router reads the query only", got.query.Get("browser"))
			}
			if got.body != "" {
				t.Errorf("browser was also put in a body the endpoint never parses: %q", got.body)
			}
		})
	}
}

// snap must behave as it does for pinchtab_navigate, including for a call that named
// no tab: the snapshot is scoped to the tab the navigation reported.
func TestHistoryToolsSnapAppendsTheSnapshot(t *testing.T) {
	for _, tool := range []string{"pinchtab_back", "pinchtab_forward", "pinchtab_reload"} {
		t.Run(tool, func(t *testing.T) {
			srv, seen := historyRecorder(t)

			result := callTool(t, tool, map[string]any{"snap": true}, srv)
			if len(*seen) != 2 {
				t.Fatalf("requests = %d, want the navigation and the snapshot", len(*seen))
			}
			snapshot := (*seen)[1]
			if snapshot.path != "/snapshot" {
				t.Fatalf("second request path = %q, want /snapshot", snapshot.path)
			}
			if got := snapshot.query.Get("tabId"); got != "ABC123" {
				t.Errorf("snapshot tabId = %q, want the tab the navigation reported", got)
			}
			if got := snapshot.query.Get("filter"); got != "interactive" {
				t.Errorf("snapshot filter = %q, want interactive", got)
			}
			if text := resultText(t, result); !strings.Contains(text, `"nodes"`) {
				t.Errorf("snap did not append the snapshot: %q", text)
			}
		})
	}
}

// Without snap there must be exactly one request, so the snapshot is opt-in rather
// than a second round-trip every client pays for.
func TestHistoryToolsMakeNoSnapshotRequestWithoutSnap(t *testing.T) {
	srv, seen := historyRecorder(t)

	callTool(t, "pinchtab_back", map[string]any{}, srv)

	if len(*seen) != 1 {
		t.Fatalf("requests = %d, want only the navigation: %+v", len(*seen), *seen)
	}
}

// The three tools must be registered, or tools/list does not advertise them and no
// client can call them however well the handler works.
func TestHistoryToolsAreRegistered(t *testing.T) {
	declared := map[string]bool{}
	for _, tool := range allTools() {
		declared[tool.Name] = true
	}
	handlers := rawHandlerMap(NewClient("http://example.invalid", ""))

	for _, name := range []string{"pinchtab_back", "pinchtab_forward", "pinchtab_reload"} {
		if !declared[name] {
			t.Errorf("%s is not in allTools(), so tools/list does not advertise it", name)
		}
		if _, ok := handlers[name]; !ok {
			t.Errorf("%s has no handler, so NewServer panics on it", name)
		}
	}
}

// browser and snap are only dangerous TOGETHER, which is why each of them having its own
// test left the combination unguarded: with browser alone there is no second request to
// mis-route, and with snap alone there is no browser to lose. Set both and the snapshot
// leg has to carry the routing too — otherwise the navigation moves the named instance
// and the snapshot comes back from the DEFAULT one, presented as the result of that
// navigation. The payload is well-formed, so nothing downstream can tell.
//
// historyRecorder is reused for pinchtab_navigate rather than duplicated: it keeps every
// request in order WITH its query, which is the axis this defect lives on. That navigate
// also sends a body is beside the point here — the assertion is about routing.
func TestSnapAndBrowserTogetherRouteBothRequestsToTheNamedInstance(t *testing.T) {
	for _, tc := range []struct {
		tool string
		args map[string]any
		path string
	}{
		{tool: "pinchtab_back", args: map[string]any{}, path: "/back"},
		{tool: "pinchtab_forward", args: map[string]any{}, path: "/forward"},
		{tool: "pinchtab_reload", args: map[string]any{}, path: "/reload"},
		{tool: "pinchtab_navigate", args: map[string]any{"url": "https://example.com"}, path: "/navigate"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			srv, seen := historyRecorder(t)

			args := map[string]any{"browser": "cloak", "snap": true}
			maps.Copy(args, tc.args)
			result := callTool(t, tc.tool, args, srv)
			if result.IsError {
				t.Fatalf("%s failed: %s", tc.tool, resultText(t, result))
			}
			if len(*seen) != 2 {
				t.Fatalf("requests = %d, want the navigation and the snapshot: %+v", len(*seen), *seen)
			}

			verb, snapshot := (*seen)[0], (*seen)[1]
			if verb.path != tc.path {
				t.Fatalf("first request path = %q, want %q", verb.path, tc.path)
			}
			if snapshot.path != "/snapshot" {
				t.Fatalf("second request path = %q, want /snapshot", snapshot.path)
			}
			if got := verb.query.Get("browser"); got != "cloak" {
				t.Errorf("%s went to browser %q, want cloak", tc.path, got)
			}
			if got := snapshot.query.Get("browser"); got != "cloak" {
				t.Errorf("the snapshot went to browser %q while %s went to cloak: the tool would answer with another instance's page as the result of this navigation", got, tc.path)
			}
		})
	}
}

// pinchtab_navigate must forward newTab so an MCP agent can OPEN a tab, not just
// manage existing ones. The server here models the /navigate newTab contract (a
// new tab on newTab:true, otherwise reuse) and /tabs, so the test drives the real
// tools end to end: list_tabs before, navigate {newTab:true}, list_tabs after,
// and asserts the count went up by one and a tabId came back. A plain navigate is
// the control — it must reuse the current tab and leave the count unchanged.
func TestHandleNavigateNewTabOpensATab(t *testing.T) {
	var tabs []string
	nextID := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/navigate":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			newTab, _ := body["newTab"].(bool)
			var id string
			if newTab || len(tabs) == 0 {
				nextID++
				id = fmt.Sprintf("tab-%d", nextID)
				tabs = append(tabs, id)
			} else {
				id = tabs[len(tabs)-1]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tabId": id, "url": body["url"], "newTab": newTab})
		case "/tabs":
			out := make([]map[string]any, len(tabs))
			for i, id := range tabs {
				out[i] = map[string]any{"id": id}
			}
			_ = json.NewEncoder(w).Encode(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	countTabs := func() int {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(resultText(t, callTool(t, "pinchtab_list_tabs", nil, srv))), &arr); err != nil {
			t.Fatalf("list_tabs is not a JSON array: %v", err)
		}
		return len(arr)
	}

	// Seed the current tab with a plain navigate, then count.
	_ = callTool(t, "pinchtab_navigate", map[string]any{"url": "https://example.com/a"}, srv)
	before := countTabs()

	r := callTool(t, "pinchtab_navigate", map[string]any{"url": "https://example.com/b", "newTab": true}, srv)
	if r.IsError {
		t.Fatalf("navigate newTab returned error: %s", resultText(t, r))
	}
	res := resultJSON(t, r)
	if id, _ := res["tabId"].(string); id == "" {
		t.Errorf("newTab navigate returned no tabId to target the new tab: %v", res)
	}
	if res["newTab"] != true {
		t.Errorf("the MCP handler did not forward newTab; the server saw newTab=%v", res["newTab"])
	}
	if after := countTabs(); after != before+1 {
		t.Fatalf("newTab did not open a tab: count %d -> %d", before, after)
	}

	// Control: a plain navigate reuses the current tab, so the count is unchanged.
	countAfterNewTab := countTabs()
	_ = callTool(t, "pinchtab_navigate", map[string]any{"url": "https://example.com/c"}, srv)
	if reuse := countTabs(); reuse != countAfterNewTab {
		t.Fatalf("a plain navigate changed the tab count: %d -> %d", countAfterNewTab, reuse)
	}
}

func TestHandleNavigateSnapStripsTheTrivialRouteButKeepsAnEscalation(t *testing.T) {
	escalated := `{"requestedProvider":"cloak","usedProvider":"ghost-chrome","escalated":true,"reason":"challenge","attempts":[{"provider":"cloak","accepted":false},{"provider":"ghost-chrome","accepted":true}]}`
	for name, tc := range map[string]struct{ route, want string }{
		"trivial":   {trivialRoute, `{"tabId":"t1"}`},
		"escalated": {escalated, `{"tabId":"t1","route":` + escalated + `}`},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/navigate":
					_, _ = w.Write([]byte(`{"tabId":"t1","route":` + tc.route + `}`))
				case "/snapshot":
					_, _ = w.Write([]byte(`{"snapshot":true}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			r := callTool(t, "pinchtab_navigate", map[string]any{"url": "https://example.com", "snap": true}, srv)
			if r.IsError {
				t.Fatalf("navigate returned error: %s", resultText(t, r))
			}
			if got, want := resultText(t, r), tc.want+"\n"+`{"snapshot":true}`; got != want {
				t.Fatalf("reply:\n got %s\nwant %s", got, want)
			}
		})
	}
}
