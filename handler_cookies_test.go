package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHandleSetCookies_InvalidJSON(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()

	b.handleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSetCookies_NoTab(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	body := `{"url":"https://example.com","cookies":[{"name":"a","value":"b"}],"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	b.handleSetCookies(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetCookies_NameFilter(t *testing.T) {

	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	req := httptest.NewRequest("GET", "/cookies?name=session_id&tabId=nonexistent", nil)
	w := httptest.NewRecorder()

	b.handleGetCookies(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] == nil {
		t.Error("expected error in response")
	}
}
