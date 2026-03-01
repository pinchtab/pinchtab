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

// MockBridge is a test implementation of the BridgeAPI interface
type MockBridge struct {
	targets        []*target.Info
	listTargetsErr string
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
	return nil
}
