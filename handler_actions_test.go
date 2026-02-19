package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHandleActions_EmptyArray(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(`{"actions": []}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.handleActions(w, req)

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
	b := newTestBridgeWithTabs()

	body := `{
		"actions": [
			{"kind": "click", "selector": "button"}
		]
	}`

	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.handleActions(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleGetCookies_NoTab(t *testing.T) {
	b := newTestBridgeWithTabs()

	req := httptest.NewRequest("GET", "/cookies", nil)
	w := httptest.NewRecorder()

	b.handleGetCookies(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleSetCookies_EmptyURL(t *testing.T) {
	b := &Bridge{}

	body := `{"cookies": [{"name": "test", "value": "123"}]}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.handleSetCookies(w, req)

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
	b := &Bridge{}

	body := `{"url": "https://example.com", "cookies": []}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.handleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for empty cookies, got %d", w.Code)
	}
}

func TestHandleStealthStatus_NoTabs(t *testing.T) {
	b := newTestBridgeWithTabs()

	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	b.handleStealthStatus(w, req)

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

	if _, ok := resp["score"].(float64); !ok {
		t.Error("expected score")
	}
}

func TestHandleFingerprintRotate_NoTab(t *testing.T) {
	b := newTestBridgeWithTabs()

	body := `{"os": "windows", "browser": "chrome"}`
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.handleFingerprintRotate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestActionRegistry_HumanActions(t *testing.T) {
	b := &Bridge{}
	b.initActionRegistry()
	registry := b.actions

	if _, ok := registry[actionHumanClick]; !ok {
		t.Error("humanClick action not registered")
	}

	if _, ok := registry[actionHumanType]; !ok {
		t.Error("humanType action not registered")
	}

	expectedActions := []string{
		actionClick, actionType, actionFill, actionPress,
		actionFocus, actionHover, actionSelect, actionScroll,
		actionHumanClick, actionHumanType,
	}

	if len(registry) != len(expectedActions) {
		t.Errorf("expected %d actions, got %d", len(expectedActions), len(registry))
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
