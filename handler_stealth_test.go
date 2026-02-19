package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestHandleStealthStatus_NoTab_ReturnsStatic(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	b.handleStealthStatus(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !searchString(body, "level") || !searchString(body, "score") {
		t.Errorf("expected level and score in response, got: %s", body)
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
	if fp.ScreenWidth == 0 || fp.ScreenHeight == 0 {
		t.Error("expected non-zero screen dimensions")
	}
	if fp.Vendor != "Google Inc." {
		t.Errorf("expected Google Inc., got %q", fp.Vendor)
	}
}

func TestGenerateFingerprint_Mac(t *testing.T) {
	fp := generateFingerprint(fingerprintRequest{OS: "mac"})
	if fp.Platform != "MacIntel" {
		t.Errorf("expected MacIntel, got %q", fp.Platform)
	}
}

func TestGenerateFingerprint_Random(t *testing.T) {
	fp := generateFingerprint(fingerprintRequest{OS: "random"})
	validPlatforms := map[string]bool{"Win32": true, "MacIntel": true}
	if !validPlatforms[fp.Platform] {
		t.Errorf("unexpected platform %q", fp.Platform)
	}
}

func TestGenerateFingerprint_UnknownOS(t *testing.T) {

	fp := generateFingerprint(fingerprintRequest{OS: "freebsd"})

	_ = fp
}

func TestGenerateFingerprint_WithBrowser(t *testing.T) {
	fp := generateFingerprint(fingerprintRequest{OS: "windows", Browser: "chrome"})
	if fp.UserAgent == "" {
		t.Error("expected non-empty user agent")
	}
}

func TestGetStealthRecommendations_AllEnabled(t *testing.T) {
	features := map[string]bool{
		"user_agent_override":   true,
		"languages_spoofed":     true,
		"webrtc_leak_prevented": true,
		"timezone_spoofed":      true,
		"canvas_noise":          true,
		"font_spoofing":         true,
	}
	recs := getStealthRecommendations(features)
	if len(recs) != 1 || recs[0] != "Stealth mode is well configured" {
		t.Errorf("expected well-configured message, got %v", recs)
	}
}

func TestGetStealthRecommendations_NoneEnabled(t *testing.T) {
	features := map[string]bool{}
	recs := getStealthRecommendations(features)
	if len(recs) != 6 {
		t.Errorf("expected 6 recommendations, got %d: %v", len(recs), recs)
	}
}

func TestGetStealthRecommendations_Partial(t *testing.T) {
	features := map[string]bool{
		"user_agent_override": true,
		"timezone_spoofed":    true,
	}
	recs := getStealthRecommendations(features)

	if len(recs) != 4 {
		t.Errorf("expected 4 recommendations, got %d: %v", len(recs), recs)
	}
}

func TestSendStealthResponse_HighScore(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	features := map[string]bool{
		"a": true, "b": true, "c": true, "d": true, "e": true,
	}
	w := httptest.NewRecorder()
	b.sendStealthResponse(w, features, "TestAgent/1.0")

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !searchString(body, `"level":"high"`) {
		t.Errorf("expected high level for 100%% score, got: %s", body)
	}
}

func TestSendStealthResponse_LowScore(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	features := map[string]bool{
		"a": true, "b": false, "c": false, "d": false, "e": false,
		"f": false, "g": false, "h": false, "i": false, "j": false,
	}
	w := httptest.NewRecorder()
	b.sendStealthResponse(w, features, "")

	body := w.Body.String()
	if !searchString(body, `"level":"minimal"`) {
		t.Errorf("expected minimal level for 10%% score, got: %s", body)
	}
}

func TestSendStealthResponse_MediumScore(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}

	features := map[string]bool{
		"a": true, "b": true, "c": true, "d": true, "e": true,
		"f": true, "g": false, "h": false, "i": false, "j": false,
	}
	w := httptest.NewRecorder()
	b.sendStealthResponse(w, features, "")

	body := w.Body.String()
	if !searchString(body, `"level":"medium"`) {
		t.Errorf("expected medium level, got: %s", body)
	}
}

func TestStaticStealthFeatures_HasEntries(t *testing.T) {
	features := staticStealthFeatures()
	if len(features) == 0 {
		t.Error("expected non-empty stealth features map")
	}
}
