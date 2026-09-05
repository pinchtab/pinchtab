package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// The two tools must be advertised in tools/list AND have a handler, or the MCP
// surface and the route catalogue drift apart on exactly the pair this card adds.
func TestConsoleAndErrorToolsAreRegistered(t *testing.T) {
	declared := map[string]bool{}
	for _, tool := range allTools() {
		declared[tool.Name] = true
	}
	handlers := rawHandlerMap(NewClient("http://example.invalid", ""))

	for _, name := range []string{"pinchtab_console", "pinchtab_errors"} {
		if !declared[name] {
			t.Errorf("%s is not in allTools(), so tools/list does not advertise it", name)
		}
		if _, ok := handlers[name]; !ok {
			t.Errorf("%s has no handler, so NewServer panics on it", name)
		}
	}
}

// A page that throws produces an /errors payload that is ALL failures. /errors is a
// channel whose content is failures, so the MCP call must report success whenever the
// HTTP call succeeded — never keying failure off the error entries in the body. If the
// funnel misread this, the one channel that says why the page died would reach the agent
// as an error, and the agent would learn nothing.
func TestErrorsToolReturnsSuccessCarryingTheErrorText(t *testing.T) {
	const thrown = "pinchtab-mcp-real-page-boom"
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<script>setTimeout(() => { throw new Error("` + thrown + `") }, 0)</script>`))
	}))
	defer page.Close()

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	browserCtx, cancelBrowser := chromedp.NewContext(alloc)
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancelTimeout()
	defer cancelBrowser()
	defer cancelAlloc()
	if err := chromedp.Run(browserCtx); err != nil {
		t.Fatalf("start test browser: %v", err)
	}
	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := bridge.New(context.Background(), browserCtx, cfg)
	tabID, _, _, err := b.CreateTab(page.URL)
	if err != nil {
		t.Fatalf("create tab with throwing page: %v", err)
	}

	mux := http.NewServeMux()
	handlers.New(b, cfg, nil, nil, nil).RegisterRoutes(mux, func() {})
	api := httptest.NewServer(mux)
	defer api.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		result := callTool(t, "pinchtab_errors", map[string]any{"tabId": tabID}, api)
		text := resultText(t, result)
		if result.IsError {
			t.Fatalf("a payload full of errors was reported as a failed call; /errors content IS failures and the call succeeded: %s", text)
		}
		if strings.Contains(text, thrown) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("successful result does not carry the error thrown by the real page: %q", text)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The same contract for /console, and the clear flag must route to the POST clear
// endpoint rather than the GET read — the CLI's `pinchtab console --clear` shape.
func TestConsoleToolReadsAndClears(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/clear") {
			_ = json.NewEncoder(w).Encode(map[string]any{"cleared": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tabId":   "tab1",
			"console": []map[string]any{{"timestamp": "2026-09-03T08:46:59Z", "level": "error", "message": "boom"}},
		})
	}))
	defer srv.Close()

	read := callTool(t, "pinchtab_console", map[string]any{"tabId": "tab1"}, srv)
	if read.IsError || !strings.Contains(resultText(t, read), "boom") {
		t.Fatalf("console read did not succeed carrying the log: %s", resultText(t, read))
	}

	cleared := callTool(t, "pinchtab_console", map[string]any{"tabId": "tab1", "clear": true}, srv)
	if cleared.IsError {
		t.Fatalf("console clear reported failure: %s", resultText(t, cleared))
	}

	if len(seen) != 2 || seen[0] != "GET /console" || seen[1] != "POST /console/clear" {
		t.Fatalf("clear did not route to the POST clear endpoint: %v", seen)
	}
}
