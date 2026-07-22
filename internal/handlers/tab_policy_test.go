package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type busyPolicyBridge struct {
	policyMockBridge
	currentURLErr error
	tabContextIDs []string
}

func (b *busyPolicyBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	b.tabContextIDs = append(b.tabContextIDs, tabID)
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (b *busyPolicyBridge) CurrentURL(context.Context) (string, error) {
	if b.currentURLErr != nil {
		return "", b.currentURLErr
	}
	return "https://example.com/report", nil
}

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
		ActionTimeout:  time.Second,
		AllowedDomains: []string{"example.com"},
		IDPI: config.IDPIConfig{
			Enabled:    true,
			StrictMode: true,
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
		ActionTimeout:  time.Second,
		AllowedDomains: []string{"example.com"},
		IDPI: config.IDPIConfig{
			Enabled:    true,
			StrictMode: false,
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
		ActionTimeout:  time.Second,
		AllowedDomains: []string{"example.com"},
		IDPI: config.IDPIConfig{
			Enabled:    true,
			StrictMode: true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/back?tabId=tab1", nil)
	w := httptest.NewRecorder()
	h.HandleBack(w, req)

	if w.Code == 403 {
		t.Fatalf("expected back to bypass current-tab policy enforcement, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExplicitTabWaitReturnsRetryableBusyWithoutLosingTab(t *testing.T) {
	b := &busyPolicyBridge{currentURLErr: context.DeadlineExceeded}
	b.mockBridge.evaluateFn = func(_ string, result any) error {
		*(result.(*bool)) = true
		return nil
	}
	h := New(b, &config.RuntimeConfig{
		AllowedDomains: []string{"example.com"},
		IDPI:           config.IDPIConfig{Enabled: true, StrictMode: true},
	}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/tabs/exact-tab/wait", nil)
	w := httptest.NewRecorder()
	h.handleWaitCore(w, req, waitRequest{TabID: "exact-tab", Text: "ready"})

	if w.Code != 503 {
		t.Fatalf("expected 503 tab_busy, got %d: %s", w.Code, w.Body.String())
	}
	var failure struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
		Details   struct {
			TabID string `json:"tabId"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode busy response: %v", err)
	}
	if failure.Code != "tab_unresponsive" || !failure.Retryable || failure.Details.TabID != "exact-tab" ||
		!strings.Contains(w.Body.String(), "activate it or open a fresh tab") {
		t.Fatalf("unexpected busy response: %+v", failure)
	}

	b.currentURLErr = nil
	retry := httptest.NewRecorder()
	h.handleWaitCore(retry, req, waitRequest{TabID: "exact-tab", Text: "ready"})
	if retry.Code != 200 {
		t.Fatalf("same explicit tab did not recover: %d %s", retry.Code, retry.Body.String())
	}
	for _, got := range b.tabContextIDs {
		if got != "exact-tab" {
			t.Fatalf("explicit tab changed across retry: %v", b.tabContextIDs)
		}
	}
}
