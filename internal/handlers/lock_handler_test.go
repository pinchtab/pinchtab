package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleTabLock(t *testing.T) {
	b := bridge.New(nil, nil, nil)
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	body, _ := json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-a", "timeoutSec": 10})
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/tab/lock", bytes.NewReader(body))
	h.HandleTabLock(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["locked"] != true {
		t.Fatalf("expected locked=true: %v", resp)
	}
	if resp["owner"] != "agent-a" {
		t.Fatalf("expected owner=agent-a: %v", resp)
	}

	body, _ = json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-b"})
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("POST", "/tab/lock", bytes.NewReader(body))
	h.HandleTabLock(w, r)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandleTabUnlock(t *testing.T) {
	b := bridge.New(nil, nil, nil)
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
	_ = b.Lock("t1", "agent-a", 10*time.Minute)

	body, _ := json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-b"})
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/tab/unlock", bytes.NewReader(body))
	h.HandleTabUnlock(w, r)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	body, _ = json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-a"})
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("POST", "/tab/unlock", bytes.NewReader(body))
	h.HandleTabUnlock(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleTabLockValidation(t *testing.T) {
	b := bridge.New(nil, nil, nil)
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	body, _ := json.Marshal(map[string]any{"tabId": "t1"})
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/tab/lock", bytes.NewReader(body))
	h.HandleTabLock(w, r)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
