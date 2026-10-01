package mcp

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// requiredArgs supplies the minimum each browser-taking tool needs to reach the
// HTTP call. It doubles as the membership list: a new tool that exposes
// `browser` fails TestEveryBrowserToolForwardsInTheQuery until it is added here,
// which is what stops the next tool from picking the body form.
var browserToolArgs = map[string]map[string]any{
	"pinchtab_navigate":         {"url": "https://example.com"},
	"pinchtab_back":             {},
	"pinchtab_forward":          {},
	"pinchtab_reload":           {},
	"pinchtab_snapshot":         {},
	"pinchtab_frame":            {"target": "#payment"},
	"pinchtab_screenshot":       {},
	"pinchtab_capture":          {},
	"pinchtab_get_text":         {},
	"pinchtab_a11y_audit":       {},
	"pinchtab_memory":           {},
	"pinchtab_memory_snapshot":  {},
	"pinchtab_memory_compare":   {"base": "heap_a", "head": "heap_b"},
	"pinchtab_click":            {"selector": "#go"},
	"pinchtab_type":             {"selector": "#q", "text": "hello"},
	"pinchtab_hover":            {"selector": "#go"},
	"pinchtab_focus":            {"selector": "#q"},
	"pinchtab_select":           {"selector": "#s", "value": "one"},
	"pinchtab_scroll":           {},
	"pinchtab_scroll_into_view": {"selector": "#go"},
	"pinchtab_fill":             {"selector": "#q", "value": "hello"},
	"pinchtab_key":              {"action": "press", "key": "Enter"},
	"pinchtab_wait":             {"for": "selector", "value": ".loaded"},
	"pinchtab_scrape":           {"url": "https://example.com"},
}

// bodyBrowserEndpoints are the endpoints whose own handler decodes a `browser`
// field and resolves the browser below the routing layer. On a single-instance
// server there is no router reading the query, so the body value must stay.
var bodyBrowserEndpoints = map[string]bool{
	"/navigate": true,
	"/action":   true,
	"/scrape":   true,
}

type recordedRequest struct {
	path  string
	query url.Values
	body  map[string]any
}

func recordingPinchTab(t *testing.T, seen *recordedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.query = r.URL.Query()
		seen.body = nil
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			var parsed map[string]any
			if json.Unmarshal(raw, &parsed) == nil {
				seen.body = parsed
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

// The routing layer reads r.URL.Query() only, so a browser sent in the body is
// served by whichever instance the router defaults to. Every tool exposing
// `browser` has to put it in the query; the handlers whose endpoint also reads a
// body browser send both.
func TestEveryBrowserToolForwardsInTheQuery(t *testing.T) {
	tools := 0
	for _, tool := range allTools() {
		if _, ok := tool.InputSchema.Properties["browser"]; !ok {
			continue
		}
		tools++
		args, known := browserToolArgs[tool.Name]
		if !known {
			t.Errorf("%s exposes a browser argument but is not covered here; add it to browserToolArgs so its forwarding is pinned", tool.Name)
			continue
		}

		t.Run(tool.Name, func(t *testing.T) {
			var seen recordedRequest
			srv := recordingPinchTab(t, &seen)
			defer srv.Close()

			call := map[string]any{"browser": "cloak"}
			maps.Copy(call, args)
			callTool(t, tool.Name, call, srv)

			if seen.path == "" {
				t.Fatalf("%s never reached the server", tool.Name)
			}
			if got := seen.query.Get("browser"); got != "cloak" {
				t.Errorf("%s sent browser=%q in the query for %s; the router reads the query only, so this call would be served by the default instance (body=%v)",
					tool.Name, got, seen.path, seen.body)
			}
			if bodyBrowserEndpoints[seen.path] {
				if got, _ := seen.body["browser"].(string); got != "cloak" {
					t.Errorf("%s dropped the body browser for %s (got %q); that endpoint resolves it itself, and a single-instance server has no router reading the query",
						tool.Name, seen.path, got)
				}
			}
		})
	}
	if tools == 0 {
		t.Fatal("no tool exposes a browser argument — this test is checking nothing")
	}
}

// Without a browser argument nothing is appended, so a single-browser
// deployment sends exactly the request it sent before.
func TestRoutingHelpersAreNoOpsWithoutABrowser(t *testing.T) {
	var seen recordedRequest
	srv := recordingPinchTab(t, &seen)
	defer srv.Close()

	callTool(t, "pinchtab_navigate", map[string]any{"url": "https://example.com"}, srv)

	if seen.query.Has("browser") {
		t.Errorf("navigate sent a browser query parameter with no browser argument: %v", seen.query)
	}
	if _, present := seen.body["browser"]; present {
		t.Errorf("navigate sent a browser body field with no browser argument: %v", seen.body)
	}
	if got, _ := seen.body["url"].(string); got != "https://example.com" {
		t.Errorf("navigate payload lost its url: %v", seen.body)
	}
}
