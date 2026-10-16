package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func TestHandleHealth_NilBridge(t *testing.T) {
	h := &Handlers{
		Bridge: nil,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if status, ok := resp["status"]; !ok || status != "error" {
		t.Errorf("expected status=error, got %v", status)
	}

	if reason, ok := resp["reason"]; !ok || reason != "bridge not initialized" {
		t.Errorf("expected reason about bridge not initialized, got %v", reason)
	}
}

func TestHandleHealth_BridgeListTargetsError(t *testing.T) {
	mockBridge := &healthTabsBridge{
		targets:        nil,
		listTargetsErr: "no CDP connection",
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if status, ok := resp["status"]; !ok || status != "error" {
		t.Errorf("expected status=error, got %v", status)
	}

	if reason, ok := resp["reason"]; !ok {
		t.Errorf("expected reason in response, got %v", reason)
	}
}

func TestHandleHealth_Success(t *testing.T) {
	mockBridge := &healthTabsBridge{
		targets: []bridge.TabTarget{
			{TargetID: "target1", URL: "https://pinchtab.com", Title: "Example"},
		},
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if status, ok := resp["status"]; !ok || status != "ok" {
		t.Errorf("expected status=ok, got %v", status)
	}

	if tabs, ok := resp["tabs"].(float64); !ok || tabs != 1 {
		t.Errorf("expected tabs=1, got %v", tabs)
	}
}

func TestHandleTabs_NilBridge(t *testing.T) {
	h := &Handlers{
		Bridge: nil,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/tabs", nil)
	w := httptest.NewRecorder()

	h.HandleTabs(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleTabs_Success(t *testing.T) {
	mockBridge := &healthTabsBridge{
		targets: []bridge.TabTarget{
			{TargetID: "tab1", URL: "https://pinchtab.com", Title: "Example", Type: "page"},
			{TargetID: "tab2", URL: "https://google.com", Title: "Google", Type: "page"},
		},
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/tabs", nil)
	w := httptest.NewRecorder()

	h.HandleTabs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	tabs, ok := resp["tabs"].([]any)
	if !ok {
		t.Fatalf("expected tabs array, got %T", resp["tabs"])
	}

	if len(tabs) != 2 {
		t.Errorf("expected 2 tabs, got %d", len(tabs))
	}
	if !mockBridge.ensureBrowserCalled {
		t.Fatal("expected tab listing to initialize the browser before enumerating existing pages")
	}
	if mockBridge.createTabCalled {
		t.Fatal("tab listing must not create a new page")
	}
}

func TestHandleTabs_TransientTabsListedOnlyWhenFlagged(t *testing.T) {
	targets := []bridge.TabTarget{
		{TargetID: "real", URL: "https://example.com/", Type: "page"},
		{TargetID: "blank", URL: "about:blank", Type: "page"},
		{TargetID: "file", URL: "file:///tmp/page.html", Type: "page"},
		{TargetID: "own-port", URL: "http://localhost:9867/dashboard", Type: "page"},
	}
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "default", query: "", want: []string{"real"}},
		{name: "flagged", query: "?" + IncludeTransientTabsQuery + "=1", want: []string{"real", "blank", "file", "own-port"}},
		{name: "flag off", query: "?" + IncludeTransientTabsQuery + "=0", want: []string{"real"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{
				Bridge: &healthTabsBridge{targets: targets, currentTabID: "real"},
				Config: &config.RuntimeConfig{Port: "9867"},
			}
			w := httptest.NewRecorder()
			h.HandleTabs(w, httptest.NewRequest("GET", "/tabs"+tc.query, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
			}
			var resp struct {
				Tabs []struct {
					ID string `json:"id"`
				} `json:"tabs"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := make([]string, 0, len(resp.Tabs))
			for _, tab := range resp.Tabs {
				got = append(got, tab.ID)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("tabs = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHandleTabs_EnsureBrowserFailureStopsBeforeEnumeration(t *testing.T) {
	mockBridge := &healthTabsBridge{ensureBrowserErr: "attach failed"}
	h := &Handlers{Bridge: mockBridge, Config: &config.RuntimeConfig{}}
	req := httptest.NewRequest("GET", "/tabs", nil)
	w := httptest.NewRecorder()

	h.HandleTabs(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected initialization failure, got %d: %s", w.Code, w.Body.String())
	}
	if !mockBridge.ensureBrowserCalled {
		t.Fatal("expected EnsureBrowser to be called")
	}
	if mockBridge.listTargetsCalled {
		t.Fatal("ListTargets must not run after initialization fails")
	}
}

func TestHandleTabs_CurrentTrackedTabIsReturnedFirst(t *testing.T) {
	mockBridge := &healthTabsBridge{
		targets: []bridge.TabTarget{
			{TargetID: "tab1", URL: "https://pinchtab.com", Title: "Example", Type: "page"},
			{TargetID: "tab2", URL: "https://google.com", Title: "Google", Type: "page"},
			{TargetID: "tab3", URL: "https://example.com", Title: "Example 2", Type: "page"},
		},
		currentTabID: "tab2",
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/tabs", nil)
	w := httptest.NewRecorder()

	h.HandleTabs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Tabs []struct {
			ID string `json:"id"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Tabs) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(resp.Tabs))
	}
	if resp.Tabs[0].ID != "tab2" {
		t.Fatalf("expected current tracked tab first, got %q", resp.Tabs[0].ID)
	}
}

func TestHandleHealth_EnsureBrowserFailure(t *testing.T) {
	mockBridge := &healthTabsBridge{
		targets:             []bridge.TabTarget{},
		ensureBrowserErr:    "failed to start browser: connection refused",
		ensureBrowserCalled: false,
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	// Should fail before calling ListTargets because ensureBrowser fails first
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if status, ok := resp["status"]; !ok || status != "error" {
		t.Errorf("expected status=error, got %v", status)
	}

	if !mockBridge.ensureBrowserCalled {
		t.Error("expected ensureBrowser to be called before ListTargets")
	}

	reason, ok := resp["reason"].(string)
	if !ok || !contains(reason, "browser") {
		t.Errorf("expected error reason mentioning browser, got %v", reason)
	}
}

func TestHandleHealth_EnsureBrowserSuccess(t *testing.T) {
	mockBridge := &healthTabsBridge{
		targets: []bridge.TabTarget{
			{TargetID: "target1", URL: "https://pinchtab.com", Title: "Example"},
		},
		ensureBrowserCalled: false,
		ensureBrowserErr:    "",
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if !mockBridge.ensureBrowserCalled {
		t.Error("expected ensureBrowser to be called before ListTargets")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if status, ok := resp["status"]; !ok || status != "ok" {
		t.Errorf("expected status=ok, got %v", status)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type healthTabsBridge struct {
	targets             []bridge.TabTarget
	listTargetsErr      string
	ensureBrowserCalled bool
	ensureBrowserErr    string
	currentTabID        string
	draining            bool
	retryAfter          time.Duration
	listTargetsCalled   bool
	createTabCalled     bool
}

func (m *healthTabsBridge) ListTargets() ([]bridge.TabTarget, error) {
	m.listTargetsCalled = true
	if m.listTargetsErr != "" {
		return nil, fmt.Errorf("%s", m.listTargetsErr)
	}
	return m.targets, nil
}

func (m *healthTabsBridge) BrowserContext() context.Context {
	return context.Background()
}

func (m *healthTabsBridge) CurrentTabID() string {
	return m.currentTabID
}

func (m *healthTabsBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if tabID == "" && m.currentTabID != "" {
		return bridge.NewTabHandle(context.Background()), m.currentTabID, nil
	}
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (m *healthTabsBridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	m.createTabCalled = true
	return "", context.Background(), func() {}, nil
}

func (m *healthTabsBridge) CloseTab(tabID string) error {
	return nil
}

func (m *healthTabsBridge) FocusTab(tabID string) error {
	return nil
}

func (m *healthTabsBridge) ScheduleIdleLifecycle(tabID string) {}

func (m *healthTabsBridge) CancelIdleLifecycle(tabID string) {}

func (m *healthTabsBridge) GetRefCache(tabID string) *bridge.RefCache {
	return nil
}

func (m *healthTabsBridge) SetRefCache(tabID string, cache *bridge.RefCache) {
}

func (m *healthTabsBridge) DeleteRefCache(tabID string) {
}

func (m *healthTabsBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	return nil, nil
}

func (m *healthTabsBridge) AvailableActions() []string {
	return nil
}

func (m *healthTabsBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	return nil
}

func (m *healthTabsBridge) Lock(tabID, owner string, ttl time.Duration) error {
	return nil
}

func (m *healthTabsBridge) Unlock(tabID, owner string) error {
	return nil
}

func (m *healthTabsBridge) EnsureBrowser(cfg *config.RuntimeConfig) error {
	m.ensureBrowserCalled = true
	if m.ensureBrowserErr != "" {
		return fmt.Errorf("%s", m.ensureBrowserErr)
	}
	return nil
}

func (m *healthTabsBridge) RunningBrowser() (string, bool) { return "", false }

func (m *healthTabsBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	if m.ensureBrowserErr != "" {
		return fmt.Errorf("%s", m.ensureBrowserErr)
	}
	return nil
}

func (m *healthTabsBridge) RestartStatus() (bool, time.Duration) {
	return m.draining, m.retryAfter
}

func (m *healthTabsBridge) StealthStatus() *stealth.Status {
	return &stealth.Status{
		Level:         stealth.LevelLight,
		LaunchMode:    stealth.LaunchModeUninitialized,
		WebdriverMode: stealth.WebdriverModeNativeBaseline,
		Flags:         map[string]bool{},
		Capabilities:  map[string]bool{},
		TabOverrides:  map[string]bool{"fingerprintRotateActive": false},
	}
}

func (m *healthTabsBridge) GetAggregatedMemoryMetrics() (*bridgeobserve.MemoryMetrics, error) {
	return &bridgeobserve.MemoryMetrics{MemoryMB: 50, Renderers: 3}, nil
}

func (m *healthTabsBridge) GetCrashLogs() []string {
	return nil
}

func (m *healthTabsBridge) NetworkMonitor() *bridge.NetworkMonitor {
	return nil
}

func (m *healthTabsBridge) AddRouteRule(tabID string, rule bridge.RouteRule) error { return nil }

func (m *healthTabsBridge) RemoveRouteRule(tabID, pattern string) (int, error) { return 0, nil }

func (m *healthTabsBridge) ListRouteRules(tabID string) ([]bridge.RouteRule, error) { return nil, nil }

func (m *healthTabsBridge) GetDialogManager() *bridgetabs.DialogManager {
	return bridgetabs.NewDialogManager()
}

func (m *healthTabsBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
}

func (m *healthTabsBridge) GetConsoleLogs(tabID string, limit int) []types.ConsoleLogEntry {
	return nil
}

func (m *healthTabsBridge) ClearConsoleLogs(tabID string) {}

func (m *healthTabsBridge) GetErrorLogs(tabID string, limit int) []types.ErrorLogEntry {
	return nil
}

func (m *healthTabsBridge) ClearErrorLogs(tabID string) {}

func (m *healthTabsBridge) ClearCache(ctx context.Context) error {
	return nil
}

func (m *healthTabsBridge) CanClearCache(ctx context.Context) (bool, error) {
	return true, nil
}

func (m *healthTabsBridge) ClearCookies(ctx context.Context) error {
	return nil
}

func (m *healthTabsBridge) Evaluate(ctx context.Context, expression string, result any, opts bridge.EvalOpts) error {
	return nil
}

func (m *healthTabsBridge) CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error) {
	return nil, nil
}

func (m *healthTabsBridge) StartScreencast(ctx context.Context, opts bridge.ScreencastOpts) (*bridge.ScreencastStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *healthTabsBridge) Navigate(ctx context.Context, url string, params bridge.NavigateParams) (*bridge.NavigateResult, error) {
	return nil, nil
}

func (m *healthTabsBridge) Snapshot(ctx context.Context, tabID string, filter string, params bridge.ContentParams) (*bridge.SnapshotResult, error) {
	return nil, nil
}

func (m *healthTabsBridge) Text(ctx context.Context, tabID string, params bridge.ContentParams) (*bridge.TextResult, error) {
	return nil, nil
}

func (m *healthTabsBridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return fmt.Errorf("not implemented")
}

func (m *healthTabsBridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts bridge.EvalOpts) error {
	return fmt.Errorf("not implemented")
}

func (m *healthTabsBridge) DescribeNode(ctx context.Context, backendNodeID int64) (*bridge.NodeInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *healthTabsBridge) SetViewport(ctx context.Context, params bridge.ViewportParams) error {
	return nil
}

func (m *healthTabsBridge) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return nil
}

func (m *healthTabsBridge) SetEmulatedMedia(ctx context.Context, feature, value string) error {
	return nil
}

func (m *healthTabsBridge) SetNetworkConditions(ctx context.Context, params bridge.NetworkConditions) error {
	return nil
}

func (m *healthTabsBridge) SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error {
	return nil
}

func (m *healthTabsBridge) GetCookies(ctx context.Context, urls []string) ([]bridge.CookieData, error) {
	return nil, nil
}

func (m *healthTabsBridge) SetCookie(ctx context.Context, params bridge.SetCookieParams) error {
	return nil
}

func (m *healthTabsBridge) CurrentURL(ctx context.Context) (string, error) {
	return "", nil
}

func (m *healthTabsBridge) CurrentTitle(ctx context.Context) (string, error) {
	return "", nil
}

func (m *healthTabsBridge) PrintToPDF(ctx context.Context, params bridge.PDFParams) ([]byte, error) {
	return nil, nil
}

func (m *healthTabsBridge) SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error {
	return nil
}

func (m *healthTabsBridge) ResolveSelectorToNodeID(ctx context.Context, selector string, refCache *bridge.RefCache, frameID string) (int64, error) {
	return 0, nil
}

func (m *healthTabsBridge) DownloadURL(ctx context.Context, dlURL string, opts bridge.DownloadOpts) (*bridge.DownloadResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *healthTabsBridge) EnableFetchWithAuth(ctx context.Context) error                      { return nil }
func (m *healthTabsBridge) DisableFetch(ctx context.Context) error                             { return nil }
func (m *healthTabsBridge) ListenAuthRequired(ctx context.Context, handler func(string, bool)) {}
func (m *healthTabsBridge) ContinueWithAuth(ctx context.Context, requestID, u, p string) error {
	return nil
}
func (m *healthTabsBridge) ContinueRequest(ctx context.Context, requestID string) error            { return nil }
func (m *healthTabsBridge) SetFetchPauseSuppressed(tabID string, v bool)                           {}
func (m *healthTabsBridge) GoBack(ctx context.Context) (bool, error)                               { return false, nil }
func (m *healthTabsBridge) GoForward(ctx context.Context) (bool, error)                            { return false, nil }
func (m *healthTabsBridge) Reload(ctx context.Context) error                                       { return nil }
func (m *healthTabsBridge) WaitVisible(ctx context.Context, selector string) error                 { return nil }
func (m *healthTabsBridge) EnableNetwork(ctx context.Context) error                                { return nil }
func (m *healthTabsBridge) ListenNetworkEvents(ctx context.Context, h2 bridge.NetworkEventHandler) {}
func (m *healthTabsBridge) SetRawCookie(ctx context.Context, p bridge.RawSetCookieParams) error {
	return nil
}
func (m *healthTabsBridge) GetRawCookies(ctx context.Context) ([]bridge.RawCookie, error) {
	return nil, nil
}
func (m *healthTabsBridge) SetUserAgentOverride(ctx context.Context, p bridge.UserAgentOverrideParams) error {
	return nil
}
func (m *healthTabsBridge) SetLocaleOverride(ctx context.Context, locale string) error { return nil }
func (m *healthTabsBridge) SetTimezoneOverride(ctx context.Context, tz string) error   { return nil }
func (m *healthTabsBridge) SetDeviceMetricsOverride(ctx context.Context, p bridge.DeviceMetricsOverrideParams) error {
	return nil
}
func (m *healthTabsBridge) AddScriptToEvaluateOnNewDocument(ctx context.Context, source string) (string, error) {
	return "", nil
}

type mockBridgeDisconnected struct {
	mockBridge
}

func (m *mockBridgeDisconnected) ListTargets() ([]bridge.TabTarget, error) {
	return nil, fmt.Errorf("disconnected")
}

func TestHandleHealth_Disconnected_Returns503(t *testing.T) {
	mb := &mockBridgeDisconnected{}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.HandleHealth(w, req)
	if w.Code != 503 {
		t.Errorf("expected 503 for disconnected browser, got %d", w.Code)
	}
}

func TestHandleHealth_Draining_Returns503WithRetryAfter(t *testing.T) {
	mb := &healthTabsBridge{draining: true, retryAfter: 1500 * time.Millisecond}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
}

func TestHandleTabs_Draining_Returns503WithRetryAfter(t *testing.T) {
	mb := &healthTabsBridge{draining: true, retryAfter: time.Second}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs", nil)
	w := httptest.NewRecorder()

	h.HandleTabs(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestHandleHealth_Connected_Returns200(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.HandleHealth(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 for connected browser, got %d", w.Code)
	}
}

func TestHandleHealth_Response(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.HandleHealth(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestHandleHealth_IncludesFailureAndCrashDiagnostics(t *testing.T) {
	ResetObservabilityForTests()
	bridge.ResetCrashMonitoringForTests()
	recordFailureEvent(FailureEvent{
		Time:      time.Now(),
		RequestID: "req_123",
		Method:    "GET",
		Path:      "/tabs/bad",
		Status:    500,
		Type:      "http_error",
	})
	bridge.RecordCrashForTests(bridge.CrashEvent{
		Time:   time.Now(),
		Reason: "target crashed",
	})

	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.HandleHealth(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, ok := resp["failures"]; !ok {
		t.Fatal("expected failures diagnostics in /health response")
	}
	if _, ok := resp["crashes"]; !ok {
		t.Fatal("expected crashes diagnostics in /health response")
	}
}

// GET /tabs/{id}/metrics reports the owning INSTANCE's process-tree memory. The tab
// id selects the instance and the 404 for an unknown tab, never the measurement, so
// two live tabs of one instance must answer identically — that is the documented
// behaviour (docs/reference/metrics.md, docs/guides/memory-monitoring.md), not a bug.
//
// The day a genuinely per-target reading exists, this is the test that has to change,
// and changing it is the decision to publish a narrower scope under this path.
func TestTabMetricsReportTheInstanceAggregateForEveryTabOfOneInstance(t *testing.T) {
	h := New(&healthTabsBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	read := func(path string) map[string]any {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v (body %s)", path, err, w.Body.String())
		}
		return body
	}

	first := read("/tabs/tab-one/metrics")
	second := read("/tabs/tab-two/metrics")
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Errorf("two tabs of one instance reported different memory (%v vs %v); this endpoint measures the browser process tree, so a per-tab difference would mean the scope changed",
			first, second)
	}

	// The scope is checkable rather than merely asserted: the same reading is what
	// this instance publishes for itself at /metrics, which is the layer the
	// reference table names.
	instance, ok := read("/metrics")["memory"].(map[string]any)
	if !ok {
		t.Fatalf("the instance /metrics carried no memory block; the comparison below would be vacuous")
	}
	if fmt.Sprint(first) != fmt.Sprint(instance) {
		t.Errorf("the tab endpoint reported %v while the instance reports %v; the tab path must serve the instance aggregate, not a second source",
			first, instance)
	}
}
