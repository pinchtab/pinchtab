package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestCredentialStoreGetReturnsCopy(t *testing.T) {
	cs := newCredentialStore()
	cs.Set("tab1", &credentialPair{Username: "u", Password: "p"})

	got, ok := cs.Get("tab1")
	if !ok {
		t.Fatal("Get: credentials not found")
	}
	// Mutating the returned value must not affect the stored credentials.
	got.Username = "tampered"

	again, _ := cs.Get("tab1")
	if again.Username != "u" {
		t.Errorf("Get leaked store state: stored username = %q, want \"u\"", again.Username)
	}
}

func TestCredentialStoreRemoveTab(t *testing.T) {
	cs := newCredentialStore()
	cs.Set("tab1", &credentialPair{Username: "u", Password: "p"})
	if !cs.MarkListenerIfAbsent("tab1") {
		t.Fatal("MarkListenerIfAbsent: expected first mark to install")
	}

	cs.RemoveTab("tab1")

	if _, ok := cs.Get("tab1"); ok {
		t.Error("RemoveTab left credentials for the closed tab")
	}
	// Listener flag must be cleared too: a future tab reusing the ID should be
	// treated as needing a fresh listener.
	if !cs.MarkListenerIfAbsent("tab1") {
		t.Error("RemoveTab did not clear the listeners flag")
	}
}

func TestHandleSetCredentials_Valid(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/emulation/credentials", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSetCredentials(w, req)

	// The mock bridge returns a cancelled context, so chromedp.Run will fail
	// with a context error. We accept 200 or 500 (CDP failure on mock).
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d: %s", w.Code, w.Body.String())
	}

	if w.Code == 200 {
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "applied" {
			t.Fatalf("expected status=applied, got %v", resp["status"])
		}
		if resp["username"] != "admin" {
			t.Fatalf("expected username=admin, got %v", resp["username"])
		}
	}
}

func TestHandleSetCredentials_ClearCredentials(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"username":""}`
	req := httptest.NewRequest(http.MethodPost, "/emulation/credentials", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSetCredentials(w, req)

	// Accept 200 (cleared) or 500 (CDP failure on mock context).
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d: %s", w.Code, w.Body.String())
	}

	if w.Code == 200 {
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "cleared" {
			t.Fatalf("expected status=cleared, got %v", resp["status"])
		}
	}
}

func TestHandleSetCredentials_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/emulation/credentials", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleSetCredentials(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetCredentials_MissingUsername(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/emulation/credentials", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSetCredentials(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for missing username, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabSetCredentials_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"username":"admin","password":"secret","tabId":"tab_other"}`
	req := httptest.NewRequest(http.MethodPost, "/tabs/tab_abc/emulation/credentials", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabSetCredentials(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for tab ID mismatch, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabSetCredentials_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/tabs/tab_abc/emulation/credentials", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabSetCredentials(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404 for nonexistent tab, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCredentialStoreListenerDedup(t *testing.T) {
	cs := newCredentialStore()

	if !cs.MarkListenerIfAbsent("tab1") {
		t.Fatal("first MarkListenerIfAbsent should return true")
	}

	if cs.MarkListenerIfAbsent("tab1") {
		t.Fatal("second MarkListenerIfAbsent should return false")
	}

	// Delete clears credentials but preserves listener tracking, because the
	// chromedp listener is bound to the tab context and survives clear/re-set.
	cs.Set("tab1", &credentialPair{Username: "u", Password: "p"})
	cs.Delete("tab1")

	if cs.MarkListenerIfAbsent("tab1") {
		t.Fatal("MarkListenerIfAbsent should return false after Delete (listener persists)")
	}
	if _, ok := cs.Get("tab1"); ok {
		t.Fatal("Delete should clear credentials")
	}
}

func TestHandleSetCredentials_EmptyPassword(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"username":"admin","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/emulation/credentials", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSetCredentials(w, req)

	// Empty password is valid — some auth schemes allow it.
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d: %s", w.Code, w.Body.String())
	}
}
