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

// PIN-420: the tool must forward awaitPromise into the /evaluate body so an MCP
// agent can resolve a Promise, matching HTTP and CLI. Without the arg the key must
// be absent, so the server keeps the un-awaited behaviour (the {}+hint path).
func TestHandleEvalForwardsAwaitPromise(t *testing.T) {
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastBody = nil
		_ = json.NewDecoder(r.Body).Decode(&lastBody)
		_, _ = w.Write([]byte(`{"result":42}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "")

	callSharedTool(t, c, "pinchtab_eval", map[string]any{"expression": "Promise.resolve(42)", "awaitPromise": true})
	if lastBody["awaitPromise"] != true {
		t.Fatalf("awaitPromise=true was not forwarded to /evaluate: body=%v", lastBody)
	}

	callSharedTool(t, c, "pinchtab_eval", map[string]any{"expression": "document.title"})
	if _, present := lastBody["awaitPromise"]; present {
		t.Fatalf("awaitPromise must be absent when not requested, so the un-awaited hint path is unchanged: body=%v", lastBody)
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

func TestHandleFindAmbiguousTieWithholdsSelector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"best_ref":   "e3",
			"confidence": "ambiguous",
			"tied_refs":  []string{"e3", "e1", "e2"},
		})
	}))
	defer srv.Close()

	req := mcp.CallToolRequest{}
	req.Params.Name = "pinchtab_find"
	req.Params.Arguments = map[string]any{"query": "Add button for Blue cup"}
	result, err := handleFind(NewClient(srv.URL, ""))(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	resp := resultJSON(t, result)
	if _, ok := resp["selector"]; ok {
		t.Errorf("selector = %v, want none for an ambiguous tie", resp["selector"])
	}
	hint, _ := resp["nextActionHint"].(string)
	if !strings.Contains(hint, "Ambiguous") || !strings.Contains(hint, "e1") || strings.Contains(hint, "Use selector") {
		t.Errorf("nextActionHint = %q, want an ambiguity hint naming the tied refs", hint)
	}
}
