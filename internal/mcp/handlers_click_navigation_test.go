package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// isError:true is the signal every agent loop branches on, and a click that
// navigated used to raise it after the click had already worked — so the natural
// reaction, retry, re-clicked on the page the first click had reached.
func TestClickThatNavigatedIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"clicked":     true,
				"navigated":   true,
				"url":         "https://www.iana.org/help/example-domains",
				"previousUrl": "https://example.com/",
				"refsStale":   true,
			},
		})
	}))
	defer srv.Close()

	result := callTool(t, "pinchtab_click", map[string]any{"ref": "e1"}, srv)

	if result.IsError {
		t.Fatalf("a click that ran and moved the page reported isError: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "https://www.iana.org/help/example-domains") {
		t.Errorf("result = %q; the caller cannot see where the click landed", text)
	}
}
