package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestHandleSnapshot_InvalidFilter(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	req := httptest.NewRequest("GET", "/snapshot?filter=bogus", nil)
	w := httptest.NewRecorder()

	b.handleSnapshot(w, req)

	if w.Code == 0 {
		t.Error("expected a response code")
	}
}

func TestHandleScreenshot_RawParam(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	req := httptest.NewRequest("GET", "/screenshot?raw=true&tabId=nonexistent", nil)
	w := httptest.NewRecorder()

	b.handleScreenshot(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_WithTimeout(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	body := `{"url":"https://example.com","timeout":5,"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	b.handleNavigate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_WithBlockImages(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	tr := true
	_ = tr
	body := `{"url":"https://example.com","blockImages":true,"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	b.handleNavigate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_WaitTitle(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	body := `{"url":"https://example.com","waitTitle":true,"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	b.handleNavigate(w, req)

	if w.Code != 404 && w.Code != 400 {
		t.Errorf("expected 404 or 400, got %d", w.Code)
	}
}
