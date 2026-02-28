package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type failMockBridge struct {
	bridge.BridgeAPI
}

func (m *failMockBridge) TabContext(tabID string) (context.Context, string, error) {
	return nil, "", fmt.Errorf("tab not found")
}

func (m *failMockBridge) ListTargets() ([]*target.Info, error) {
	return nil, fmt.Errorf("list targets failed")
}

func (m *failMockBridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	return nil
}

func TestHandleActions_EmptyArray(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(`{"actions": []}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "actions array is empty" {
		t.Errorf("expected empty array error, got %v", resp["error"])
	}
}

func TestHandleActions_NoTabError(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{
		"actions": [
			{"kind": "click", "selector": "button"}
		]
	}`

	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleGetCookies_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/cookies", nil)
	w := httptest.NewRecorder()

	h.HandleGetCookies(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleSetCookies_EmptyURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"cookies": [{"name": "test", "value": "123"}]}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "url is required" {
		t.Errorf("expected url required error, got %v", resp["error"])
	}
}

func TestHandleSetCookies_EmptyCookies(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"url": "https://example.com", "cookies": []}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for empty cookies, got %d", w.Code)
	}
}

func TestHandleStealthStatus_NoTabs(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	h.HandleStealthStatus(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	features, ok := resp["features"].(map[string]interface{})
	if !ok {
		t.Error("expected features map")
	}

	if len(features) == 0 {
		t.Error("expected non-empty features")
	}
}

func TestHandleFingerprintRotate_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"os": "windows", "browser": "chrome"}`
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleFingerprintRotate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestCountSuccessful(t *testing.T) {
	results := []actionResult{
		{Success: true},
		{Success: false},
		{Success: true},
		{Success: true},
	}

	count := countSuccessful(results)
	if count != 3 {
		t.Errorf("expected 3 successful, got %d", count)
	}
}
