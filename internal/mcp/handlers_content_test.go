package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

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

func TestHandleFindAddsSelectorHints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/find" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"best_ref": "e7",
			"score":    0.91,
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	h := handleFind(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_find"
	req.Params.Arguments = map[string]any{"query": "search button"}

	result, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	resp := resultJSON(t, result)
	if got, _ := resp["bestRef"].(string); got != "e7" {
		t.Fatalf("bestRef = %q, want e7", got)
	}
	if got, _ := resp["selector"].(string); got != "e7" {
		t.Fatalf("selector = %q, want e7", got)
	}
	if _, ok := resp["nextActionHint"].(string); !ok {
		t.Fatal("expected nextActionHint in response")
	}
}
