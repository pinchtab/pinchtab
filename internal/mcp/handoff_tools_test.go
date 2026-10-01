package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func decodeToolJSON(t *testing.T, name string, args map[string]any, api *httptest.Server) (map[string]any, bool) {
	t.Helper()
	result := callTool(t, name, args, api)
	text := resultText(t, result)
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("%s answered non-JSON %q", name, text)
	}
	return body, result.IsError
}

func TestAnAgentRecoversFromAHandoffRefusalThroughTheMCPToolsAlone(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<button id="go" onclick="this.textContent='clicked'">go</button>`))
	}))
	defer page.Close()

	browserCtx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	defer cancelTimeout()
	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(browserCtx, cfg)
	tabID, _, _, err := b.CreateTab(page.URL)
	if err != nil {
		t.Fatalf("create tab: %v", err)
	}
	mux := http.NewServeMux()
	handlers.New(b, cfg, nil, nil, nil).RegisterRoutes(mux, func() {})
	api := httptest.NewServer(mux)
	defer api.Close()
	click := map[string]any{"tabId": tabID, "selector": "#go"}

	paused, isErr := decodeToolJSON(t, "pinchtab_handoff", map[string]any{"tabId": tabID, "reason": "captcha_manual"}, api)
	if isErr || paused["status"] != "paused_handoff" || paused["reason"] != "captcha_manual" {
		t.Fatalf("pinchtab_handoff = %v (error=%v), want paused_handoff with the reason", paused, isErr)
	}

	refused, isErr := decodeToolJSON(t, "pinchtab_click", click, api)
	if !isErr || refused["code"] != "tab_paused_handoff" {
		t.Fatalf("click on a paused tab = %v (error=%v), want the tab_paused_handoff refusal", refused, isErr)
	}

	status, isErr := decodeToolJSON(t, "pinchtab_handoff_status", map[string]any{"tabId": tabID}, api)
	if isErr || status["status"] != "paused_handoff" || status["reason"] != "captcha_manual" {
		t.Fatalf("pinchtab_handoff_status while paused = %v (error=%v), want paused_handoff", status, isErr)
	}

	resumed, isErr := decodeToolJSON(t, "pinchtab_resume", map[string]any{"tabId": tabID, "status": "completed"}, api)
	if isErr || resumed["status"] != "active" || resumed["resumeStatus"] != "completed" {
		t.Fatalf("pinchtab_resume = %v (error=%v), want active carrying the status note", resumed, isErr)
	}

	status, isErr = decodeToolJSON(t, "pinchtab_handoff_status", map[string]any{"tabId": tabID}, api)
	if isErr || status["status"] != "active" {
		t.Fatalf("pinchtab_handoff_status after resume = %v (error=%v), want active", status, isErr)
	}

	clicked, isErr := decodeToolJSON(t, "pinchtab_click", click, api)
	if isErr {
		t.Fatalf("click after resume refused: %v", clicked)
	}
	text := resultText(t, callTool(t, "pinchtab_get_text", map[string]any{"tabId": tabID}, api))
	if !strings.Contains(text, "clicked") {
		t.Fatalf("page text after the resumed click = %q, want the button relabelled by the click", text)
	}
}

func TestHandoffToolsRequireATabAndForwardTheirParameters(t *testing.T) {
	type seen struct {
		method, path string
		body         map[string]any
	}
	var last seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = seen{method: r.Method, path: r.URL.EscapedPath(), body: map[string]any{}}
		_ = json.NewDecoder(r.Body).Decode(&last.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tabId":"t1","status":"active"}`))
	}))
	defer srv.Close()

	for _, tc := range []struct {
		tool   string
		args   map[string]any
		method string
		path   string
		body   map[string]any
	}{
		{"pinchtab_handoff", map[string]any{"tabId": "t/1", "reason": " login_required ", "timeoutMs": float64(1500)}, http.MethodPost, "/tabs/t%2F1/handoff", map[string]any{"reason": "login_required", "timeoutMs": float64(1500)}},
		{"pinchtab_handoff", map[string]any{"tabId": "t1"}, http.MethodPost, "/tabs/t1/handoff", map[string]any{}},
		{"pinchtab_resume", map[string]any{"tabId": "t1", "status": "completed"}, http.MethodPost, "/tabs/t1/resume", map[string]any{"status": "completed"}},
		{"pinchtab_handoff_status", map[string]any{"tabId": "t1"}, http.MethodGet, "/tabs/t1/handoff", map[string]any{}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			last = seen{}
			if result := callTool(t, tc.tool, tc.args, srv); result.IsError {
				t.Fatalf("%s errored: %s", tc.tool, resultText(t, result))
			}
			if last.method != tc.method || last.path != tc.path {
				t.Fatalf("%s sent %s %s, want %s %s", tc.tool, last.method, last.path, tc.method, tc.path)
			}
			for k, want := range tc.body {
				if last.body[k] != want {
					t.Errorf("%s body[%s] = %v, want %v", tc.tool, k, last.body[k], want)
				}
			}
			for k := range last.body {
				if _, declared := tc.body[k]; !declared {
					t.Errorf("%s sent undeclared body field %s=%v", tc.tool, k, last.body[k])
				}
			}
			without := callTool(t, tc.tool, map[string]any{}, srv)
			if !without.IsError || last.method != tc.method {
				t.Fatalf("%s without a tabId did not refuse before forwarding", tc.tool)
			}
		})
	}
}

func TestEveryToolNameInTheAgentFacingDocsResolves(t *testing.T) {
	registered := map[string]bool{}
	for _, tool := range allTools() {
		registered[tool.Name] = true
	}
	name := regexp.MustCompile(`pinchtab_[a-z][a-z0-9_]*`)
	root := filepath.Join("..", "..")
	for _, doc := range []string{
		"docs/mcp.md",
		"docs/reference/mcp-tools.md",
		"skills/pinchtab/references/mcp.md",
		"skills/pinchtab-mcp/SKILL.md",
	} {
		data, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, hit := range name.FindAllString(string(data), -1) {
			found[strings.TrimRight(hit, "_")] = true
		}
		if len(found) == 0 {
			t.Fatalf("%s names no tool; the census is checking nothing", doc)
		}
		for _, tool := range []string{"pinchtab_handoff", "pinchtab_resume", "pinchtab_handoff_status"} {
			if !found[tool] {
				t.Errorf("%s does not document %s", doc, tool)
			}
		}
		for hit := range found {
			if !registered[hit] {
				t.Errorf("%s names %s, which is not a registered tool", doc, hit)
			}
		}
	}
}
