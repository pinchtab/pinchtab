package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
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
		Config: &config.RuntimeConfig{CdpURL: "ws://localhost:9222"},
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
		targets: []*target.Info{
			{TargetID: "target1", URL: "https://example.com", Title: "Example"},
		},
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{CdpURL: "ws://localhost:9222"},
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
		targets: []*target.Info{
			{TargetID: "tab1", URL: "https://example.com", Title: "Example", Type: "page"},
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

// TestHandleHealth_EnsureChromeFailure verifies /health returns 503 when Chrome initialization fails
func TestHandleHealth_EnsureChromeFailure(t *testing.T) {
	mockBridge := &MockBridge{
		targets:            []*target.Info{},
		ensureChromeErr:    "failed to start Chrome: connection refused",
		ensureChromeCalled: false,
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	// Should fail before calling ListTargets because ensureChrome fails first
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

	// Verify ensureChrome was actually called
	if !mockBridge.ensureChromeCalled {
		t.Error("expected ensureChrome to be called before ListTargets")
	}

	// Verify error message mentions chrome initialization
	reason, ok := resp["reason"].(string)
	if !ok || !contains(reason, "chrome") {
		t.Errorf("expected error reason mentioning chrome, got %v", reason)
	}
}

// TestHandleHealth_EnsureChromeSuccess verifies /health calls ensureChrome and then checks ListTargets
func TestHandleHealth_EnsureChromeSuccess(t *testing.T) {
	mockBridge := &MockBridge{
		targets: []*target.Info{
			{TargetID: "target1", URL: "https://example.com", Title: "Example"},
		},
		ensureChromeCalled: false,
		ensureChromeErr:    "", // No error
	}

	h := &Handlers{
		Bridge: mockBridge,
		Config: &config.RuntimeConfig{CdpURL: "ws://localhost:9222"},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify ensureChrome was called
	if !mockBridge.ensureChromeCalled {
		t.Error("expected ensureChrome to be called before ListTargets")
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
	targets            []*target.Info
	listTargetsErr     string
	ensureChromeCalled bool
	ensureChromeErr    string
}

func (m *MockBridge) ListTargets() ([]*target.Info, error) {
	if m.listTargetsErr != "" {
		return nil, fmt.Errorf("%s", m.listTargetsErr)
	}
	return m.targets, nil
}

func (m *MockBridge) BrowserContext() context.Context {
	return context.Background()
}

func (m *MockBridge) TabContext(tabID string) (context.Context, string, error) {
	return context.Background(), tabID, nil
}

func (m *MockBridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	return "", context.Background(), func() {}, nil
}

func (m *MockBridge) CloseTab(tabID string) error {
	return nil
}

func (m *MockBridge) GetRefCache(tabID string) *bridge.RefCache {
	return nil
}

func (m *MockBridge) SetRefCache(tabID string, cache *bridge.RefCache) {
}

func (m *MockBridge) DeleteRefCache(tabID string) {
}

func (m *MockBridge) RegisterHashTab(hashID, rawCDPID string, ctx context.Context, cancel context.CancelFunc) {
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

func (m *MockBridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	m.ensureChromeCalled = true
	if m.ensureChromeErr != "" {
		return fmt.Errorf("%s", m.ensureChromeErr)
	}
	return nil
}
