package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

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
	if !strings.Contains(text, "waited") {
		t.Errorf("expected waited, got %s", text)
	}
}

// browser must be forwarded as a query param (routing is query-based), not the
// body — otherwise browser=cloak is silently dropped by the router.
func TestHandleWaitForSelectorForwardsBrowser(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_wait_for_selector", map[string]any{
		"selector": ".loaded",
		"browser":  "cloak",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "query") || !strings.Contains(text, "browser") || !strings.Contains(text, "cloak") {
		t.Errorf("expected forwarded browser=cloak in query, got %s", text)
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
