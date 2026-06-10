package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// TestHandleHealth_NilBridge verifies health endpoint returns 503 when bridge is nil
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

// TestHandleHealth_BridgeListTargetsError verifies health returns 503 when ListTargets fails
func TestHandleHealth_BridgeListTargetsError(t *testing.T) {
	// Create a mock bridge that returns an error
	mockBridge := &MockBridge{
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

// TestHandleHealth_Success verifies health returns 200 when everything works
func TestHandleHealth_Success(t *testing.T) {
	// Create a mock bridge that returns targets
	mockBridge := &MockBridge{
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

// TestHandleTabs_NilBridge verifies tabs endpoint returns 503 when bridge is nil
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

// TestHandleTabs_Success verifies tabs endpoint returns tab list when bridge works
func TestHandleTabs_Success(t *testing.T) {
	mockBridge := &MockBridge{
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
}

func TestHandleTabs_CurrentTrackedTabIsReturnedFirst(t *testing.T) {
	mockBridge := &MockBridge{
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

// TestHandleHealth_EnsureBrowserFailure verifies /health returns 503 when browser initialization fails
func TestHandleHealth_EnsureBrowserFailure(t *testing.T) {
	mockBridge := &MockBridge{
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

	// Verify ensureBrowser was actually called
	if !mockBridge.ensureBrowserCalled {
		t.Error("expected ensureBrowser to be called before ListTargets")
	}

	// Verify error message mentions browser initialization
	reason, ok := resp["reason"].(string)
	if !ok || !contains(reason, "browser") {
		t.Errorf("expected error reason mentioning browser, got %v", reason)
	}
}

// TestHandleHealth_EnsureBrowserSuccess verifies /health calls ensureBrowser and then checks ListTargets
func TestHandleHealth_EnsureBrowserSuccess(t *testing.T) {
	mockBridge := &MockBridge{
		targets: []bridge.TabTarget{
			{TargetID: "target1", URL: "https://pinchtab.com", Title: "Example"},
		},
		ensureBrowserCalled: false,
		ensureBrowserErr:    "", // No error
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

	// Verify ensureBrowser was called
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

// contains is a simple helper to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// MockBridge is a test implementation of the BridgeAPI interface
type MockBridge struct {
	targets             []bridge.TabTarget
	listTargetsErr      string
	ensureBrowserCalled bool
	ensureBrowserErr    string
	currentTabID        string
	draining            bool
	retryAfter          time.Duration
}

func (m *MockBridge) ListTargets() ([]bridge.TabTarget, error) {
	if m.listTargetsErr != "" {
		return nil, fmt.Errorf("%s", m.listTargetsErr)
	}
	return m.targets, nil
}

func (m *MockBridge) BrowserContext() context.Context {
	return context.Background()
}

func (m *MockBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if tabID == "" && m.currentTabID != "" {
		return bridge.NewTabHandle(context.Background()), m.currentTabID, nil
	}
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (m *MockBridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	return "", context.Background(), func() {}, nil
}

func (m *MockBridge) CloseTab(tabID string) error {
	return nil
}

func (m *MockBridge) FocusTab(tabID string) error {
	return nil
}

func (m *MockBridge) ScheduleAutoClose(tabID string) {}

func (m *MockBridge) CancelAutoClose(tabID string) {}

func (m *MockBridge) GetRefCache(tabID string) *bridge.RefCache {
	return nil
}

func (m *MockBridge) SetRefCache(tabID string, cache *bridge.RefCache) {
}

func (m *MockBridge) DeleteRefCache(tabID string) {
}

func (m *MockBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	return nil, nil
}

func (m *MockBridge) AvailableActions() []string {
	return nil
}

func (m *MockBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	return nil
}

func (m *MockBridge) Lock(tabID, owner string, ttl time.Duration) error {
	return nil
}

func (m *MockBridge) Unlock(tabID, owner string) error {
	return nil
}

func (m *MockBridge) EnsureBrowser(cfg *config.RuntimeConfig) error {
	m.ensureBrowserCalled = true
	if m.ensureBrowserErr != "" {
		return fmt.Errorf("%s", m.ensureBrowserErr)
	}
	return nil
}

func (m *MockBridge) RunningBrowser() (string, bool) { return "", false }

func (m *MockBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	if m.ensureBrowserErr != "" {
		return fmt.Errorf("%s", m.ensureBrowserErr)
	}
	return nil
}

func (m *MockBridge) RestartStatus() (bool, time.Duration) {
	return m.draining, m.retryAfter
}

func (m *MockBridge) StealthStatus() *stealth.Status {
	return &stealth.Status{
		Level:         stealth.LevelLight,
		LaunchMode:    stealth.LaunchModeUninitialized,
		WebdriverMode: stealth.WebdriverModeNativeBaseline,
		Flags:         map[string]bool{},
		Capabilities:  map[string]bool{},
		TabOverrides:  map[string]bool{"fingerprintRotateActive": false},
	}
}

func (m *MockBridge) GetMemoryMetrics(tabID string) (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{JSHeapUsedMB: 10, JSHeapTotalMB: 20}, nil
}

func (m *MockBridge) GetBrowserMemoryMetrics() (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{JSHeapUsedMB: 50, JSHeapTotalMB: 100}, nil
}

func (m *MockBridge) GetAggregatedMemoryMetrics() (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{JSHeapUsedMB: 50, JSHeapTotalMB: 100, Nodes: 500}, nil
}

func (m *MockBridge) GetCrashLogs() []string {
	return nil
}

func (m *MockBridge) NetworkMonitor() *bridge.NetworkMonitor {
	return nil
}

func (m *MockBridge) AddRouteRule(tabID string, rule bridge.RouteRule) error { return nil }

func (m *MockBridge) RemoveRouteRule(tabID, pattern string) (int, error) { return 0, nil }

func (m *MockBridge) ListRouteRules(tabID string) ([]bridge.RouteRule, error) { return nil, nil }

func (m *MockBridge) GetDialogManager() *bridge.DialogManager {
	return bridge.NewDialogManager()
}

func (m *MockBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
}

func (m *MockBridge) GetConsoleLogs(tabID string, limit int) []bridge.LogEntry {
	return nil
}

func (m *MockBridge) ClearConsoleLogs(tabID string) {}

func (m *MockBridge) GetErrorLogs(tabID string, limit int) []bridge.ErrorEntry {
	return nil
}

func (m *MockBridge) ClearErrorLogs(tabID string) {}

func (m *MockBridge) ClearCache(ctx context.Context) error {
	return nil
}

func (m *MockBridge) CanClearCache(ctx context.Context) (bool, error) {
	return true, nil
}

func (m *MockBridge) ClearCookies(ctx context.Context) error {
	return nil
}

func (m *MockBridge) Evaluate(ctx context.Context, expression string, result any, opts bridge.EvalOpts) error {
	return nil
}

func (m *MockBridge) CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error) {
	return nil, nil
}

func (m *MockBridge) StartScreencast(ctx context.Context, opts bridge.ScreencastOpts) (*bridge.ScreencastStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockBridge) Navigate(ctx context.Context, url string, params bridge.NavigateParams) (*bridge.NavigateResult, error) {
	return nil, nil
}

func (m *MockBridge) Snapshot(ctx context.Context, tabID string, filter string, params bridge.ContentParams) (*bridge.SnapshotResult, error) {
	return nil, nil
}

func (m *MockBridge) Text(ctx context.Context, tabID string, params bridge.ContentParams) (*bridge.TextResult, error) {
	return nil, nil
}

func (m *MockBridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return fmt.Errorf("not implemented")
}

func (m *MockBridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts bridge.EvalOpts) error {
	return fmt.Errorf("not implemented")
}

func (m *MockBridge) DescribeNode(ctx context.Context, backendNodeID int64) (*bridge.NodeInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockBridge) SetViewport(ctx context.Context, params bridge.ViewportParams) error {
	return nil
}

func (m *MockBridge) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return nil
}

func (m *MockBridge) SetEmulatedMedia(ctx context.Context, feature, value string) error {
	return nil
}

func (m *MockBridge) SetNetworkConditions(ctx context.Context, params bridge.NetworkConditions) error {
	return nil
}

func (m *MockBridge) SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error {
	return nil
}

func (m *MockBridge) GetCookies(ctx context.Context, urls []string) ([]bridge.CookieData, error) {
	return nil, nil
}

func (m *MockBridge) SetCookie(ctx context.Context, params bridge.SetCookieParams) error {
	return nil
}

func (m *MockBridge) CurrentURL(ctx context.Context) (string, error) {
	return "", nil
}

func (m *MockBridge) CurrentTitle(ctx context.Context) (string, error) {
	return "", nil
}

func (m *MockBridge) PrintToPDF(ctx context.Context, params bridge.PDFParams) ([]byte, error) {
	return nil, nil
}

func (m *MockBridge) SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error {
	return nil
}

func (m *MockBridge) ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error) {
	return 0, nil
}

func (m *MockBridge) DownloadURL(ctx context.Context, dlURL string, opts bridge.DownloadOpts) (*bridge.DownloadResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockBridge) EnableFetchWithAuth(ctx context.Context) error                          { return nil }
func (m *MockBridge) DisableFetch(ctx context.Context) error                                 { return nil }
func (m *MockBridge) ListenAuthRequired(ctx context.Context, handler func(string, bool))     {}
func (m *MockBridge) ContinueWithAuth(ctx context.Context, requestID, u, p string) error     { return nil }
func (m *MockBridge) ContinueRequest(ctx context.Context, requestID string) error            { return nil }
func (m *MockBridge) GoBack(ctx context.Context) (bool, error)                               { return false, nil }
func (m *MockBridge) GoForward(ctx context.Context) (bool, error)                            { return false, nil }
func (m *MockBridge) Reload(ctx context.Context) error                                       { return nil }
func (m *MockBridge) WaitVisible(ctx context.Context, selector string) error                 { return nil }
func (m *MockBridge) EnableNetwork(ctx context.Context) error                                { return nil }
func (m *MockBridge) ListenNetworkEvents(ctx context.Context, h2 bridge.NetworkEventHandler) {}
func (m *MockBridge) SetRawCookie(ctx context.Context, p bridge.RawSetCookieParams) error    { return nil }
func (m *MockBridge) GetRawCookies(ctx context.Context) ([]bridge.RawCookie, error)          { return nil, nil }
func (m *MockBridge) SetUserAgentOverride(ctx context.Context, p bridge.UserAgentOverrideParams) error {
	return nil
}
func (m *MockBridge) SetLocaleOverride(ctx context.Context, locale string) error { return nil }
func (m *MockBridge) SetTimezoneOverride(ctx context.Context, tz string) error   { return nil }
func (m *MockBridge) SetDeviceMetricsOverride(ctx context.Context, p bridge.DeviceMetricsOverrideParams) error {
	return nil
}
func (m *MockBridge) AddScriptToEvaluateOnNewDocument(ctx context.Context, source string) (string, error) {
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
	mb := &MockBridge{draining: true, retryAfter: 1500 * time.Millisecond}
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
	mb := &MockBridge{draining: true, retryAfter: time.Second}
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
	resetObservabilityForTests()
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
