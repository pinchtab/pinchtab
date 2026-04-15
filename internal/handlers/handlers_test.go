package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type mockBridge struct {
	bridge.BridgeAPI
	failTab          bool
	createTabURLs    []string
	lastConsoleLimit int
	lastErrorLimit   int
	fingerprintTabs  map[string]bool
	frameScopes      map[string]bridge.FrameScope
	ensureChromeErr  error
	ensureChromeCall int
}

func (m *mockBridge) TabContext(tabID string) (context.Context, string, error) {
	if m.failTab {
		return nil, "", fmt.Errorf("tab not found")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, "tab1", nil
}

func (m *mockBridge) ListTargets() ([]*target.Info, error) {
	return []*target.Info{{TargetID: "tab1", Type: "page"}}, nil
}

func (m *mockBridge) AvailableActions() []string {
	return []string{bridge.ActionClick, bridge.ActionType}
}

func (m *mockBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}

func (m *mockBridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	m.createTabURLs = append(m.createTabURLs, url)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately - no browser spawned
	return "tab_abc12345", ctx, cancel, nil
}

func (m *mockBridge) CloseTab(tabID string) error {
	if tabID == "fail" {
		return fmt.Errorf("close failed")
	}
	return nil
}

func (m *mockBridge) FocusTab(tabID string) error {
	if tabID == "fail" {
		return fmt.Errorf("tab not found")
	}
	return nil
}

func (m *mockBridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	m.ensureChromeCall++
	return m.ensureChromeErr
}

func (m *mockBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

func (m *mockBridge) DeleteRefCache(tabID string) {}

func (m *mockBridge) TabLockInfo(tabID string) *bridge.LockInfo { return nil }

func (m *mockBridge) GetMemoryMetrics(tabID string) (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{JSHeapUsedMB: 10}, nil
}

func (m *mockBridge) GetBrowserMemoryMetrics() (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{JSHeapUsedMB: 50}, nil
}

func (m *mockBridge) GetAggregatedMemoryMetrics() (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{JSHeapUsedMB: 50, Nodes: 500}, nil
}

func (m *mockBridge) GetCrashLogs() []string {
	return nil
}

func (m *mockBridge) NetworkMonitor() *bridge.NetworkMonitor {
	return nil
}

func (m *mockBridge) GetDialogManager() *bridge.DialogManager {
	return bridge.NewDialogManager()
}

func (m *mockBridge) GetConsoleLogs(tabID string, limit int) []bridge.LogEntry {
	m.lastConsoleLimit = limit
	return nil
}

func (m *mockBridge) ClearConsoleLogs(tabID string) {}

func (m *mockBridge) GetErrorLogs(tabID string, limit int) []bridge.ErrorEntry {
	m.lastErrorLimit = limit
	return nil
}

func (m *mockBridge) ClearErrorLogs(tabID string) {}

func (m *mockBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
}

func (m *mockBridge) GetFrameScope(tabID string) (bridge.FrameScope, bool) {
	if m.frameScopes == nil {
		return bridge.FrameScope{}, false
	}
	scope, ok := m.frameScopes[tabID]
	return scope, ok && scope.Active()
}

func (m *mockBridge) SetFrameScope(tabID string, scope bridge.FrameScope) {
	if m.frameScopes == nil {
		m.frameScopes = make(map[string]bridge.FrameScope)
	}
	m.frameScopes[tabID] = scope
}

func (m *mockBridge) ClearFrameScope(tabID string) {
	delete(m.frameScopes, tabID)
}

func (m *mockBridge) SetFingerprintRotateActive(tabID string, active bool) {
	if m.fingerprintTabs == nil {
		m.fingerprintTabs = make(map[string]bool)
	}
	m.fingerprintTabs[tabID] = active
}

func (m *mockBridge) FingerprintRotateActive(tabID string) bool {
	return m.fingerprintTabs != nil && m.fingerprintTabs[tabID]
}

func TestHandlers(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("GET", "/help", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 from /help, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "paths") {
		t.Fatalf("expected /help response to include paths (now alias for openapi.json)")
	}

	req = httptest.NewRequest("GET", "/openapi.json", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 from /openapi.json, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "openapi") {
		t.Fatalf("expected /openapi.json response to include openapi")
	}
	if !strings.Contains(w.Body.String(), "/browser/restart") {
		t.Fatalf("expected /openapi.json response to include /browser/restart")
	}

	req = httptest.NewRequest("GET", "/metrics", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 from /metrics, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "metrics") {
		t.Fatalf("expected /metrics response to include metrics")
	}
}

func TestHelpIncludesSecurityStatus(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/help", nil)
	w := httptest.NewRecorder()
	h.HandleOpenAPI(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 from /help, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "x-pinchtab-security") {
		t.Fatalf("expected /help response to include security status")
	}
	if !strings.Contains(w.Body.String(), "security.allowEvaluate") {
		t.Fatalf("expected /help response to include locked setting names")
	}
}

func TestOpenAPIIncludesSensitiveEndpointStatus(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	h.HandleOpenAPI(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 from /openapi.json, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"x-pinchtab-security\"") {
		t.Fatalf("expected /openapi.json response to include security metadata")
	}
	if !strings.Contains(w.Body.String(), "\"x-pinchtab-enabled\":true") {
		t.Fatalf("expected /openapi.json response to mark enabled sensitive endpoints")
	}
}

func TestOpenAPIIncludesEvaluateAwaitPromiseSchema(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	h.HandleOpenAPI(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 from /openapi.json, got %d", w.Code)
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal openapi: %v", err)
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected paths object, got %T", doc["paths"])
	}
	evaluatePath, ok := paths["/evaluate"].(map[string]any)
	if !ok {
		t.Fatalf("expected /evaluate path, got %T", paths["/evaluate"])
	}
	post, ok := evaluatePath["post"].(map[string]any)
	if !ok {
		t.Fatalf("expected /evaluate POST operation, got %T", evaluatePath["post"])
	}
	requestBody, ok := post["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody, got %T", post["requestBody"])
	}
	content := requestBody["content"].(map[string]any)
	appJSON := content["application/json"].(map[string]any)
	schema := appJSON["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	awaitPromise, ok := properties["awaitPromise"].(map[string]any)
	if !ok {
		t.Fatalf("expected awaitPromise property, got %T", properties["awaitPromise"])
	}
	if awaitPromise["type"] != "boolean" {
		t.Fatalf("expected awaitPromise type boolean, got %#v", awaitPromise["type"])
	}
}

func TestHandleTabMetricsReturns404ForUnknownTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("GET", "/tabs/invalid_tab_id/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from /tabs/{id}/metrics for unknown tab, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tab not found") {
		t.Fatalf("expected not-found response body, got %q", w.Body.String())
	}
}

func TestHandleNavigate(t *testing.T) {
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	cfg := &config.RuntimeConfig{}
	m := &mockBridge{}
	h := New(m, cfg, nil, nil, nil)

	// 1. Valid POST request
	body := `{"url": "https://pinchtab.com"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)
	// Even with mock context, it might fail inside chromedp.Run if no browser is attached,
	// but we're testing the handler logic around it.
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
	}

	// 2. Valid GET request (ergonomic alias path style)
	req = httptest.NewRequest("GET", "/nav?url=https%3A%2F%2Fpinchtab.com", nil)
	w = httptest.NewRecorder()
	h.HandleNavigate(w, req)
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status for GET navigate %d: %s", w.Code, w.Body.String())
	}

	// 3. Missing URL
	req = httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	h.HandleNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}

	if len(m.createTabURLs) == 0 {
		t.Fatalf("expected CreateTab to be called for new-tab navigate")
	}
	if m.createTabURLs[0] != "" {
		t.Fatalf("expected HandleNavigate to create a blank tab first, got %q", m.createTabURLs[0])
	}
}

func TestHandleTab(t *testing.T) {
	m := &mockBridge{}
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	// New Tab
	body := `{"action": "new", "url": "about:blank"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d", w.Code)
	}
	if len(m.createTabURLs) == 0 {
		t.Fatalf("expected CreateTab to be called for action=new")
	}
	if m.createTabURLs[0] != "" {
		t.Fatalf("expected HandleTab to create a blank tab first, got %q", m.createTabURLs[0])
	}

	// Close Tab
	body = `{"action": "close", "tabId": "tab1"}`
	req = httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w = httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetErrorLogs_ClampsLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    string
		expected int
	}{
		{name: "negative", limit: "-5", expected: 0},
		{name: "too_large", limit: "1001", expected: 1000},
		{name: "in_range", limit: "25", expected: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockBridge{}
			h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

			req := httptest.NewRequest("GET", "/errors?limit="+tt.limit, nil)
			w := httptest.NewRecorder()
			h.HandleGetErrorLogs(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if m.lastErrorLimit != tt.expected {
				t.Fatalf("expected limit %d, got %d", tt.expected, m.lastErrorLimit)
			}
		})
	}
}

func TestHandleGetConsoleLogs_ClampsLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    string
		expected int
	}{
		{name: "negative", limit: "-5", expected: 0},
		{name: "too_large", limit: "1001", expected: 1000},
		{name: "in_range", limit: "25", expected: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockBridge{}
			h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

			req := httptest.NewRequest("GET", "/console?limit="+tt.limit, nil)
			w := httptest.NewRecorder()
			h.HandleGetConsoleLogs(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if m.lastConsoleLimit != tt.expected {
				t.Fatalf("expected limit %d, got %d", tt.expected, m.lastConsoleLimit)
			}
		})
	}
}

func TestHandleGetConsoleLogs_BlocksWhenCachedTabPolicyIsBlocked(t *testing.T) {
	b := &policyMockBridge{
		state: bridge.TabPolicyState{
			CurrentURL: "https://evil.example.net",
			Threat:     true,
			Blocked:    true,
			Reason:     `domain "evil.example.net" is not in the allowed list`,
			UpdatedAt:  time.Now(),
		},
		hasState: true,
	}
	h := New(b, &config.RuntimeConfig{
		AllowedDomains: []string{"example.com"},
		IDPI: config.IDPIConfig{
			Enabled:    true,
			StrictMode: true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/console?tabId=tab1", nil)
	w := httptest.NewRecorder()
	h.HandleGetConsoleLogs(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetErrorLogs_BlocksWhenCachedTabPolicyIsBlocked(t *testing.T) {
	b := &policyMockBridge{
		state: bridge.TabPolicyState{
			CurrentURL: "https://evil.example.net",
			Threat:     true,
			Blocked:    true,
			Reason:     `domain "evil.example.net" is not in the allowed list`,
			UpdatedAt:  time.Now(),
		},
		hasState: true,
	}
	h := New(b, &config.RuntimeConfig{
		AllowedDomains: []string{"example.com"},
		IDPI: config.IDPIConfig{
			Enabled:    true,
			StrictMode: true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/errors?tabId=tab1", nil)
	w := httptest.NewRecorder()
	h.HandleGetErrorLogs(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabFocus(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	t.Run("focus success", func(t *testing.T) {
		body := `{"action": "focus", "tabId": "tab1"}`
		req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		h.HandleTab(w, req)
		if w.Code != 200 {
			t.Errorf("expected 200, got %d", w.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["focused"] != true {
			t.Error("expected focused=true")
		}
		if resp["tabId"] != "tab1" {
			t.Errorf("expected tabId=tab1, got %v", resp["tabId"])
		}
	})

	t.Run("focus missing tabId", func(t *testing.T) {
		body := `{"action": "focus"}`
		req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		h.HandleTab(w, req)
		if w.Code != 400 {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("focus not found", func(t *testing.T) {
		body := `{"action": "focus", "tabId": "fail"}`
		req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		h.HandleTab(w, req)
		if w.Code != 404 {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		body := `{"action": "invalid"}`
		req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		h.HandleTab(w, req)
		if w.Code != 400 {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestRoutesRegistration(t *testing.T) {
	b := &mockBridge{}
	cfg := &config.RuntimeConfig{}
	h := New(b, cfg, nil, nil, nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func() {})

	tests := []struct {
		method string
		path   string
		code   int
	}{
		{"GET", "/health", 200},
		{"GET", "/tabs", 200},
		{"POST", "/browser/restart", 200},
		{"POST", "/navigate", 400}, // missing body
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tt.code {
			t.Errorf("%s %s expected %d, got %d", tt.method, tt.path, tt.code, w.Code)
		}
	}
}

func TestEvaluateRouteLockedByDefault(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403 when evaluate is disabled, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "security.allowEvaluate") {
		t.Fatalf("expected evaluate lock response to include the setting name, got %s", w.Body.String())
	}
}

func TestEvaluateRouteRegisteredWhenEnabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected evaluate route to be active, got %d", w.Code)
	}
}

func TestSensitiveTabRouteLockedByDefault(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("POST", "/tabs/tab1/evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 when tab evaluate is disabled, got %d", w.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if payload["code"] != "evaluate_disabled" {
		t.Fatalf("expected evaluate_disabled code, got %v", payload["code"])
	}
}
