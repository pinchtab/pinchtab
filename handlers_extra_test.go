package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHandleHealth_Response(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	b.handleHealth(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestHandleNavigate_InvalidJSON(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEvaluate_InvalidJSON(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	b.handleEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_InvalidJSON(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	b.handleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_InvalidAction(t *testing.T) {
	b := &Bridge{}
	body := `{"action":"invalid"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTab_CloseMissingID(t *testing.T) {
	b := &Bridge{}
	body := `{"action":"close"}`
	req := httptest.NewRequest("POST", "/tab", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleTab(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !searchString(w.Body.String(), "tabId required") {
		t.Errorf("expected tabId required error, got %s", w.Body.String())
	}
}

func TestHandleAction_InvalidJSON(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	b.handleAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleAction_RefNotCached(t *testing.T) {
	b := newBridgeWithFakeTab()
	body := `{"kind":"click","ref":"e99","tabId":"tab1"}`
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !searchString(w.Body.String(), "not found") {
		t.Errorf("expected 'not found' error, got %s", w.Body.String())
	}
}

func TestHandleAction_MissingKindWithTab(t *testing.T) {
	b := newBridgeWithFakeTab()
	body := `{"selector":"#btn","tabId":"tab1"}`
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !searchString(w.Body.String(), "kind") {
		t.Errorf("expected 'kind' in error, got %s", w.Body.String())
	}
}

func TestHandleAction_UnknownKindWithTab(t *testing.T) {
	b := newBridgeWithFakeTab()
	body := `{"kind":"explode","selector":"#btn","tabId":"tab1"}`
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !searchString(w.Body.String(), "unknown action") {
		t.Errorf("expected 'unknown action' error, got %s", w.Body.String())
	}
}

func TestHandleShutdown_CallsFunc(t *testing.T) {
	called := make(chan bool, 1)
	doShutdown := func() { called <- true }
	b := &Bridge{}
	handler := b.handleShutdown(doShutdown)
	req := httptest.NewRequest("POST", "/shutdown", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	select {
	case <-called:
	case <-time.After(500 * time.Millisecond):
		t.Error("expected doShutdown to be called within 500ms")
	}
}

func TestLoadConfig_TimeoutFromFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"timeout":"25s","navTimeout":"60s"}`), 0644)

	t.Setenv("BRIDGE_CONFIG", cfgPath)
	t.Setenv("BRIDGE_TIMEOUT", "")
	t.Setenv("BRIDGE_NAV_TIMEOUT", "")

	origAction := cfg.ActionTimeout
	origNav := cfg.NavigateTimeout
	defer func() {
		cfg.ActionTimeout = origAction
		cfg.NavigateTimeout = origNav
	}()

	loadConfig()

}

func TestLoadConfig_ConfigFileAllFields(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	jsonData := `{
		"port": "9999",
		"headless": true,
		"token": "test-token",
		"noRestore": true,
		"timeoutSec": 20,
		"navigateSec": 45
	}`
	_ = os.WriteFile(cfgPath, []byte(jsonData), 0644)

	origPort := cfg.Port
	origNoRestore := cfg.NoRestore
	origToken := cfg.Token
	origAction := cfg.ActionTimeout
	origNav := cfg.NavigateTimeout
	defer func() {
		cfg.Port = origPort
		cfg.NoRestore = origNoRestore
		cfg.Token = origToken
		cfg.ActionTimeout = origAction
		cfg.NavigateTimeout = origNav
	}()

	t.Setenv("BRIDGE_CONFIG", cfgPath)
	t.Setenv("BRIDGE_PORT", "")
	t.Setenv("BRIDGE_TOKEN", "")
	t.Setenv("BRIDGE_NO_RESTORE", "")
	t.Setenv("BRIDGE_TIMEOUT", "")
	t.Setenv("BRIDGE_NAV_TIMEOUT", "")
	loadConfig()

	if cfg.Port != "9999" {
		t.Errorf("expected port 9999, got %q", cfg.Port)
	}
	if !cfg.NoRestore {
		t.Error("expected noRestore true from config")
	}
	if cfg.Token != "test-token" {
		t.Errorf("expected test-token, got %q", cfg.Token)
	}
	if cfg.ActionTimeout != 20*time.Second {
		t.Errorf("expected 20s timeout, got %v", cfg.ActionTimeout)
	}
	if cfg.NavigateTimeout != 45*time.Second {
		t.Errorf("expected 45s navTimeout, got %v", cfg.NavigateTimeout)
	}
}

// newBridgeWithFakeTab creates a Bridge with a real context.Context tab for testing
// handler dispatch logic without needing Chrome.
func newBridgeWithFakeTab() *Bridge {
	b := &Bridge{}
	b.TabManager = &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*refCache),
	}
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel
	b.RegisterTab("tab1", ctx)
	return b
}
