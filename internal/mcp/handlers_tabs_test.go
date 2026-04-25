package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
