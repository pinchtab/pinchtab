package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleDialog_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDialog_MissingAction(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDialog_InvalidAction(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"action":"invalid"}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDialog_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"action":"accept"}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDialog_NoPendingDialog(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: 5e9}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"action":"accept"}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	// Should return 400 because no dialog is pending
	if w.Code != 400 {
		t.Errorf("expected 400 for no pending dialog, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDialog_AcceptAction(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: 5e9}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"action":"accept"}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	// With no pending dialog, should get 400
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Error("expected error in response")
	}
}

func TestHandleDialog_DismissAction(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: 5e9}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"action":"dismiss"}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabDialog_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//dialog", bytes.NewReader([]byte(`{"action":"accept"}`)))
	w := httptest.NewRecorder()
	h.HandleTabDialog(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabDialog_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/dialog", bytes.NewReader([]byte(`{"tabId":"tab_other","action":"accept"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabDialog(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabDialog_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{ActionTimeout: 5e9}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/dialog", bytes.NewReader([]byte(`{"action":"accept"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabDialog(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDialog_AcceptWithText(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: 5e9}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"action":"accept","text":"hello"}`)))
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)
	// No pending dialog, so 400
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDialogManagerFromMockBridge(t *testing.T) {
	mb := &mockBridge{}
	dm := mb.GetDialogManager()
	if dm == nil {
		t.Fatal("expected non-nil DialogManager from mockBridge")
	}

	// Verify it works
	dm.SetPending("tab1", &bridge.DialogState{Type: "alert", Message: "test"})
	got := dm.GetPending("tab1")
	if got == nil || got.Type != "alert" {
		t.Errorf("expected alert dialog, got %+v", got)
	}
}
