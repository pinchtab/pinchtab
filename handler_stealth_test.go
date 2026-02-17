package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestHandleStealthStatus_NoTab_Returns200(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	b.handleStealthStatus(w, req)

	// With no tab, returns static analysis (200)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleStealthStatus_NonexistentTab(t *testing.T) {
	b := &Bridge{}
	b.tabs = make(map[string]*TabEntry)
	req := httptest.NewRequest("GET", "/stealth/status?tabId=nonexistent", nil)
	w := httptest.NewRecorder()

	b.handleStealthStatus(w, req)

	// With explicit nonexistent tabId, still returns static (no tab found falls through)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleFingerprintRotate_InvalidJSON(t *testing.T) {
	b := &Bridge{}
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()

	b.handleFingerprintRotate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGenerateFingerprint_Windows(t *testing.T) {
	fp := generateFingerprint(fingerprintRequest{OS: "windows"})
	if fp.Platform != "Win32" {
		t.Errorf("expected Win32, got %q", fp.Platform)
	}
	if fp.UserAgent == "" {
		t.Error("expected non-empty user agent")
	}
	if fp.ScreenWidth == 0 {
		t.Error("expected non-zero screen width")
	}
}

func TestStaticStealthFeatures_HasEntries(t *testing.T) {
	features := staticStealthFeatures()
	if len(features) == 0 {
		t.Error("expected non-empty stealth features map")
	}
}

func TestGetStealthRecommendations_AllGood(t *testing.T) {
	features := map[string]bool{
		"navigator.webdriver":     true,
		"navigator.plugins":       true,
		"navigator.languages":     true,
		"chrome.runtime":          true,
		"permission.notification": true,
	}
	recs := getStealthRecommendations(features)
	// Recommendations include general hardening tips even when features pass
	if recs == nil {
		t.Error("expected non-nil recommendations slice")
	}
}

func TestGetStealthRecommendations_MissingFeatures(t *testing.T) {
	features := map[string]bool{
		"navigator.webdriver": false,
	}
	recs := getStealthRecommendations(features)
	if len(recs) == 0 {
		t.Error("expected recommendations for missing features")
	}
}

func TestSendStealthResponse(t *testing.T) {
	b := &Bridge{}
	features := map[string]bool{"navigator.webdriver": true}
	w := httptest.NewRecorder()

	b.sendStealthResponse(w, features, "TestAgent/1.0")

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
