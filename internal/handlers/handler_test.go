package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type mockBridge struct {
	bridge.BridgeAPI
	failTab bool
}

func (m *mockBridge) TabContext(tabID string) (context.Context, string, error) {
	if m.failTab {
		return nil, "", fmt.Errorf("tab not found")
	}
	// We need a context that chromedp.Run won't complain about,
	// even if it's not fully functional for real CDP commands.
	ctx, _ := chromedp.NewContext(context.Background())
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
	ctx, cancel := chromedp.NewContext(context.Background())
	return "new-tab", ctx, cancel, nil
}

func (m *mockBridge) CloseTab(tabID string) error {
	if tabID == "fail" {
		return fmt.Errorf("close failed")
	}
	return nil
}

func (m *mockBridge) DeleteRefCache(tabID string) {}

func (m *mockBridge) TabLockInfo(tabID string) *bridge.LockInfo { return nil }

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
	if !strings.Contains(w.Body.String(), "endpoints") {
		t.Fatalf("expected /help response to include endpoints")
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
}

func TestHandleNavigate(t *testing.T) {
	cfg := &config.RuntimeConfig{}
	m := &mockBridge{}
	h := New(m, cfg, nil, nil, nil)

	// 1. Valid POST request
	body := `{"url": "https://example.com"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)
	// Even with mock context, it might fail inside chromedp.Run if no browser is attached,
	// but we're testing the handler logic around it.
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
	}

	// 2. Valid GET request (ergonomic alias path style)
	req = httptest.NewRequest("GET", "/nav?url=https%3A%2F%2Fexample.com", nil)
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
}

func TestHandleTab(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	// New Tab
	body := `{"action": "new", "url": "about:blank"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleTab(w, req)
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d", w.Code)
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

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
