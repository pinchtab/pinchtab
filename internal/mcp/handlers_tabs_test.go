package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

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
	if !strings.Contains(text, `"/close"`) {
		t.Errorf("expected /close path, got %s", text)
	}
}

func TestHandleCloseTabAllowsMissingTabID(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_close_tab", map[string]any{}, srv)
	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, r))
	}
	resp := resultJSON(t, r)
	if resp["path"] != "/close" {
		t.Fatalf("path = %v, want /close", resp["path"])
	}
}

func TestHandleCloseTabTreatsBlankTabIDAsDefault(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_close_tab", map[string]any{"tabId": " "}, srv)
	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, r))
	}
	resp := resultJSON(t, r)
	if resp["path"] != "/close" {
		t.Fatalf("path = %v, want /close", resp["path"])
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
			"exists":  true,
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
			"exists":  true,
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

func TestHandleConnectProfileRequiresAProfileArgument(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_connect_profile", map[string]any{}, srv)
	if !r.IsError {
		t.Fatal("expected error for missing profile")
	}
}

func TestHandleConnectProfileReportsMissingAsAHumanSetupStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":    "missing",
			"exists":  false,
			"running": false,
			"status":  "missing",
			"message": `Profile "missing" does not exist. Creating and authenticating a reusable profile is a human setup step.`,
		})
	}))
	defer srv.Close()

	r := callTool(t, "pinchtab_connect_profile", map[string]any{"profile": "missing"}, srv)
	text := resultText(t, r)
	for _, want := range []string{`"exists": false`, `"status": "missing"`, "does not exist", "human setup step"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing-profile result %q does not contain %q", text, want)
		}
	}
	if strings.Contains(text, "start the profile") {
		t.Fatalf("missing-profile result suggests an agent retry: %q", text)
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

func TestHandleContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	handlers := handlerMap(c)
	h := handlers["pinchtab_health"]

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

// The set tool exists because the MCP surface was read-only while the CLI was
// delete-only, and every argument it declares must actually travel: an argument the
// schema advertises but the handler drops is worse than an absent one.
func TestHandleCookiesSetForwardsEveryDeclaredArgument(t *testing.T) {
	var seen struct {
		path string
		body map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&seen.body); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"set":1,"failed":0,"total":1}`))
	}))
	defer srv.Close()

	callTool(t, "pinchtab_cookies_set", map[string]any{
		"name":     "session",
		"value":    "abc123",
		"url":      "https://example.com/app",
		"domain":   "example.com",
		"path":     "/",
		"sameSite": "Lax",
		"secure":   true,
		"httpOnly": true,
		"expires":  float64(1893456000),
		"tabId":    "t1",
	}, srv)

	if seen.path != "/cookies" {
		t.Fatalf("path = %q, want /cookies", seen.path)
	}
	if seen.body["tabId"] != "t1" || seen.body["url"] != "https://example.com/app" {
		t.Errorf("body = %+v, want the tab and url forwarded", seen.body)
	}
	cookies, ok := seen.body["cookies"].([]any)
	if !ok || len(cookies) != 1 {
		t.Fatalf("body = %+v, want one cookie", seen.body)
	}
	cookie, _ := cookies[0].(map[string]any)
	for key, want := range map[string]any{
		"name":     "session",
		"value":    "abc123",
		"domain":   "example.com",
		"path":     "/",
		"sameSite": "Lax",
		"secure":   true,
		"httpOnly": true,
		"expires":  float64(1893456000),
	} {
		if cookie[key] != want {
			t.Errorf("cookie[%q] = %v, want %v — declared in the tool schema but not forwarded", key, cookie[key], want)
		}
	}
}

// Every argument the handler reads is declared, so the schema-derived validator sees
// it and a model can discover it.
func TestCookiesSetToolDeclaresEveryArgumentItReads(t *testing.T) {
	var tool *mcp.Tool
	for i, candidate := range allTools() {
		if candidate.Name == "pinchtab_cookies_set" {
			tool = &allTools()[i]
		}
	}
	if tool == nil {
		t.Fatal("pinchtab_cookies_set is not in allTools(), so tools/list never advertises it")
	}
	if _, ok := rawHandlerMap(NewClient("http://example.invalid", ""))["pinchtab_cookies_set"]; !ok {
		t.Fatal("pinchtab_cookies_set has no handler, so NewServer panics on it")
	}

	for _, arg := range []string{"name", "value", "url", "domain", "path", "sameSite", "secure", "httpOnly", "expires", "tabId"} {
		if _, ok := tool.InputSchema.Properties[arg]; !ok {
			t.Errorf("the handler reads %q but the schema does not declare it", arg)
		}
	}
	for _, required := range []string{"name", "value"} {
		if !slices.Contains(tool.InputSchema.Required, required) {
			t.Errorf("%q is not required, so a call omitting it reaches the server as an empty cookie", required)
		}
	}
}

// An empty value blanks a cookie; RequireString accepts it, so the tool must not treat
// it as a missing argument.
func TestHandleCookiesSetAcceptsAnEmptyValue(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"set":1,"failed":0,"total":1}`))
	}))
	defer srv.Close()

	callTool(t, "pinchtab_cookies_set", map[string]any{"name": "session", "value": ""}, srv)

	cookies, ok := body["cookies"].([]any)
	if !ok || len(cookies) != 1 {
		t.Fatalf("body = %+v, want the cookie to reach the server", body)
	}
	if cookie, _ := cookies[0].(map[string]any); cookie["value"] != "" {
		t.Errorf("cookie = %+v, want an empty value preserved", cookie)
	}
}
