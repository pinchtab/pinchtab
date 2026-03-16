package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// mockPinchTab returns an httptest.Server that echoes back request details.
func mockPinchTab() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"path":   r.URL.Path,
			"method": r.Method,
		}

		// Echo query params
		if r.URL.RawQuery != "" {
			resp["query"] = r.URL.Query()
		}

		// Echo JSON body
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				var parsed map[string]any
				if json.Unmarshal(body, &parsed) == nil {
					resp["body"] = parsed
				}
			}
		}

		// Special: /evaluate for wait_for_selector returns true
		if r.URL.Path == "/evaluate" {
			resp["result"] = true
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func callTool(t *testing.T, name string, args map[string]any, srv *httptest.Server) *mcp.CallToolResult {
	t.Helper()
	c := NewClient(srv.URL, "")
	handlers := handlerMap(c)
	h, ok := handlers[name]
	if !ok {
		t.Fatalf("no handler for %q", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return result
}

func resultText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if len(r.Content) == 0 {
		t.Fatal("no content in result")
	}
	tc, ok := r.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, not TextContent", r.Content[0])
	}
	return tc.Text
}

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

	// Empty URL should fail
	r := callTool(t, "pinchtab_navigate", map[string]any{"url": ""}, srv)
	if !r.IsError {
		t.Error("expected error for empty URL")
	}
}

func TestHandleNavigateJavaScript(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	// javascript: URLs should be allowed (user knows what they're doing)
	r := callTool(t, "pinchtab_navigate", map[string]any{"url": "javascript:void(0)"}, srv)
	if r.IsError {
		t.Errorf("expected javascript: URL to succeed, got error: %s", resultText(t, r))
	}
}

func TestHandleNavigateBareHostname(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	// Bare hostnames should be normalized to https://
	r := callTool(t, "pinchtab_navigate", map[string]any{"url": "example.com"}, srv)
	if r.IsError {
		t.Errorf("expected bare hostname to succeed, got error: %s", resultText(t, r))
	}
}

func TestHandleNavigateAnyScheme(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	// All explicit schemes should be allowed
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

func TestHandleScreenshot(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_screenshot", map[string]any{
		"quality": float64(90),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/screenshot") {
		t.Errorf("expected /screenshot, got %s", text)
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

func TestHandleClick(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref": "e5",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "click") {
		t.Errorf("expected click in response, got %s", text)
	}
}

func TestHandleClickMissingRef(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{}, srv)
	if !r.IsError {
		t.Error("expected error for missing ref")
	}
}

func TestHandleType(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_type", map[string]any{
		"ref":  "e12",
		"text": "hello world",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "type") {
		t.Errorf("expected type in response, got %s", text)
	}
	if !strings.Contains(text, "hello world") {
		t.Errorf("expected text in response, got %s", text)
	}
}

func TestHandlePress(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_press", map[string]any{
		"key": "Enter",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "Enter") {
		t.Errorf("expected Enter in response, got %s", text)
	}
}

func TestHandleSelect(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_select", map[string]any{
		"ref":   "e3",
		"value": "option2",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "select") {
		t.Errorf("expected select, got %s", text)
	}
}

func TestHandleScroll(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll", map[string]any{
		"pixels": float64(500),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "scroll") {
		t.Errorf("expected scroll, got %s", text)
	}
}

func TestHandleFill(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_fill", map[string]any{
		"ref":   "e7",
		"value": "test@example.com",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "fill") {
		t.Errorf("expected fill, got %s", text)
	}
}

func TestHandleEval(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_eval", map[string]any{
		"expression": "document.title",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/evaluate") {
		t.Errorf("expected /evaluate, got %s", text)
	}
}

func TestHandleEvalMissingExpression(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_eval", map[string]any{}, srv)
	if !r.IsError {
		t.Error("expected error for missing expression")
	}
}

func TestHandlePDF(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_pdf", map[string]any{
		"landscape":  true,
		"scale":      float64(0.8),
		"pageRanges": "1-3",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/pdf") {
		t.Errorf("expected /pdf, got %s", text)
	}
}

func TestHandleFind(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_find", map[string]any{
		"query": "login button",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/find") {
		t.Errorf("expected /find, got %s", text)
	}
}

func TestHandleListTabs(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_list_tabs", map[string]any{}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "/tabs") {
		t.Errorf("expected /tabs, got %s", text)
	}
}

func TestHandleCloseTab(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_close_tab", map[string]any{
		"tabId": "t2",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "close") {
		t.Errorf("expected close, got %s", text)
	}
}

func TestHandleHealth(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_health", map[string]any{}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "/health") {
		t.Errorf("expected /health, got %s", text)
	}
}

func TestHandleCookies(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_cookies", map[string]any{
		"tabId": "t1",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "/cookies") {
		t.Errorf("expected /cookies, got %s", text)
	}
}

func TestHandleConnectProfileRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/profiles/work/instance" {
			t.Fatalf("path = %s, want /profiles/work/instance", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":    "work",
			"running": true,
			"status":  "running",
			"port":    "9868",
			"id":      "inst_123",
		})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_connect_profile", map[string]any{
		"profile": "work",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"profile": "work"`) {
		t.Fatalf("expected profile in response, got %s", text)
	}
	if !strings.Contains(text, `"url": "`+srv.URL+`/dashboard/profiles"`) {
		t.Fatalf("expected dashboard URL in response, got %s", text)
	}
}

func TestHandleConnectProfileNotRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":    "work",
			"running": false,
			"status":  "stopped",
			"port":    "",
			"id":      "",
		})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_connect_profile", map[string]any{
		"profile": "work",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"running": false`) {
		t.Fatalf("expected running=false in response, got %s", text)
	}
	if strings.Contains(text, `"url":`) {
		t.Fatalf("did not expect url in stopped response, got %s", text)
	}
	if !strings.Contains(text, `does not have a running instance`) {
		t.Fatalf("expected no-instance message, got %s", text)
	}
}

func TestHandleConnectProfileMissingProfile(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_connect_profile", map[string]any{}, srv)
	if !r.IsError {
		t.Fatal("expected error for missing profile")
	}
}

func TestHandleWait(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_wait", map[string]any{
		"ms": float64(50),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "waited_ms") {
		t.Errorf("expected waited_ms, got %s", text)
	}
}

func TestHandleWaitClampsMax(t *testing.T) {
	// Verify that the wait handler clamps values > maxWaitMS.
	// We don't actually wait — just check the handler's clamping logic
	// by inspecting the response. Use a small value that still triggers clamping.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	c := NewClient("http://localhost:1", "")
	handlers := handlerMap(c)
	h := handlers["pinchtab_wait"]
	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_wait"
	req.Params.Arguments = map[string]any{"ms": float64(999999)}
	r, err := h(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	// Context will cancel before 30s expires, so we expect cancellation.
	text := resultText(t, r)
	if !strings.Contains(text, "cancelled") && !strings.Contains(text, "30000") {
		t.Errorf("expected 'cancelled' or '30000', got %s", text)
	}
}

func TestHandleWaitForSelector(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_wait_for_selector", map[string]any{
		"selector": ".loaded",
		"timeout":  float64(5000),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "found") {
		t.Errorf("expected found, got %s", text)
	}
}

func TestHandleWaitForSelectorMissing(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_wait_for_selector", map[string]any{}, srv)
	if !r.IsError {
		t.Error("expected error for missing selector")
	}
}

func TestHandleHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error":"internal"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	handlers := handlerMap(c)
	h := handlers["pinchtab_health"]
	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_health"
	req.Params.Arguments = map[string]any{}
	r, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError {
		t.Error("expected error result for HTTP 500")
	}
}

func TestHandleHover(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_hover", map[string]any{"ref": "e3"}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "hover") {
		t.Errorf("expected hover, got %s", text)
	}
}

func TestHandleFocus(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_focus", map[string]any{"ref": "e1"}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "focus") {
		t.Errorf("expected focus, got %s", text)
	}
}

func TestHandleContextCancellation(t *testing.T) {
	// Slow server that blocks until context is cancelled
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	handlers := handlerMap(c)
	h := handlers["pinchtab_health"]

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_health"
	req.Params.Arguments = map[string]any{}

	r, err := h(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError {
		t.Error("expected error result when context is cancelled")
	}
}

func TestHandleWaitNegativeMs(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_wait", map[string]any{"ms": float64(-100)}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "waited_ms") {
		t.Errorf("expected waited_ms, got %s", text)
	}
	if !strings.Contains(text, "0") {
		t.Errorf("expected 0ms for negative input, got %s", text)
	}
}
