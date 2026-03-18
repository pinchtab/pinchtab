package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type policyMockBridge struct {
	mockBridge
	state          bridge.TabPolicyState
	hasState       bool
	actionExecuted bool
}

func (m *policyMockBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	m.actionExecuted = true
	return map[string]any{"success": true}, nil
}

func (m *policyMockBridge) GetTabPolicyState(tabID string) (bridge.TabPolicyState, bool) {
	return m.state, m.hasState
}

func (m *policyMockBridge) SetTabPolicyState(tabID string, state bridge.TabPolicyState) {
	m.state = state
	m.hasState = true
}

func TestHandleActionBlocksWhenCachedTabPolicyIsBlocked(t *testing.T) {
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
		ActionTimeout: time.Second,
		IDPI: config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: []string{"example.com"},
			StrictMode:     true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/action", bytes.NewBufferString(`{"tabId":"tab1","kind":"click"}`))
	w := httptest.NewRecorder()
	h.HandleAction(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if b.actionExecuted {
		t.Fatal("expected action execution to be skipped")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "idpi_domain_blocked" {
		t.Fatalf("expected idpi_domain_blocked code, got %v", resp["code"])
	}
}

func TestHandleActionWarnsWhenCachedTabPolicyIsThreatOnly(t *testing.T) {
	b := &policyMockBridge{
		state: bridge.TabPolicyState{
			CurrentURL: "https://warn.example.net",
			Threat:     true,
			Blocked:    false,
			Reason:     `domain "warn.example.net" is not in the allowed list`,
			UpdatedAt:  time.Now(),
		},
		hasState: true,
	}
	h := New(b, &config.RuntimeConfig{
		ActionTimeout: time.Second,
		IDPI: config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: []string{"example.com"},
			StrictMode:     false,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/action", bytes.NewBufferString(`{"tabId":"tab1","kind":"click"}`))
	w := httptest.NewRecorder()
	h.HandleAction(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-IDPI-Warning"); got == "" {
		t.Fatal("expected X-IDPI-Warning header")
	}
	if !b.actionExecuted {
		t.Fatal("expected action execution to continue in warn mode")
	}
}

func TestHandleBackIgnoresCachedTabPolicyBlock(t *testing.T) {
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
		ActionTimeout: time.Second,
		IDPI: config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: []string{"example.com"},
			StrictMode:     true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/back?tabId=tab1", nil)
	w := httptest.NewRecorder()
	h.HandleBack(w, req)

	if w.Code == 403 {
		t.Fatalf("expected back to bypass current-tab policy enforcement, got %d: %s", w.Code, w.Body.String())
	}
}
