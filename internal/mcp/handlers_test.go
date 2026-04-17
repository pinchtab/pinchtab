package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// mockPinchTab returns an httptest.Server that echoes back request details.
func mockPinchTab() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"path":   r.URL.Path,
			"method": r.Method,
		}

		if r.URL.RawQuery != "" {
			resp["query"] = r.URL.Query()
		}

		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				var parsed map[string]any
				if json.Unmarshal(body, &parsed) == nil {
					resp["body"] = parsed
				}
			}
		}

		if r.URL.Path == "/evaluate" {
			resp["result"] = true
		}

		if r.URL.Path == "/wait" {
			resp["waited"] = true
			resp["elapsed"] = 100
			resp["match"] = "selector"
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

func resultJSON(t *testing.T, r *mcp.CallToolResult) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(resultText(t, r)), &v); err != nil {
		t.Fatalf("unmarshal result JSON: %v", err)
	}
	return v
}
