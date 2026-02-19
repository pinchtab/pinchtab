package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleTabLock(t *testing.T) {
	b := &Bridge{locks: newLockManager()}

	body, _ := json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-a", "timeoutSec": 10})
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/tab/lock", bytes.NewReader(body))
	b.handleTabLock(w, r)

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
	b.handleTabLock(w, r)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandleTabUnlock(t *testing.T) {
	b := &Bridge{locks: newLockManager()}
	_ = b.locks.Lock("t1", "agent-a", 0)

	body, _ := json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-b"})
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/tab/unlock", bytes.NewReader(body))
	b.handleTabUnlock(w, r)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	body, _ = json.Marshal(map[string]any{"tabId": "t1", "owner": "agent-a"})
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("POST", "/tab/unlock", bytes.NewReader(body))
	b.handleTabUnlock(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleTabLockValidation(t *testing.T) {
	b := &Bridge{locks: newLockManager()}

	body, _ := json.Marshal(map[string]any{"tabId": "t1"})
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/tab/lock", bytes.NewReader(body))
	b.handleTabLock(w, r)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
