package handlers

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleStealthStatus_NoTab_ReturnsStatic(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	h.HandleStealthStatus(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !searchString(body, "level") || !searchString(body, "score") {
		t.Errorf("expected level and score in response, got: %s", body)
	}
}

func TestHandleFingerprintRotate_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()

	h.HandleFingerprintRotate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGenerateFingerprint_Windows(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{ChromeVersion: "120.0.0.0"}}
	fp := h.generateFingerprint(fingerprintRequest{OS: "windows"})
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
	h := Handlers{Config: &config.RuntimeConfig{ChromeVersion: "120.0.0.0"}}
	fp := h.generateFingerprint(fingerprintRequest{OS: "mac"})
	if fp.Platform != "MacIntel" {
		t.Errorf("expected MacIntel, got %q", fp.Platform)
	}
}

func TestGenerateFingerprint_Random(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{ChromeVersion: "120.0.0.0"}}
	fp := h.generateFingerprint(fingerprintRequest{OS: "random"})
	validPlatforms := map[string]bool{"Win32": true, "MacIntel": true}
	if !validPlatforms[fp.Platform] {
		t.Errorf("unexpected platform %q", fp.Platform)
	}
}

func TestGenerateFingerprint_WithBrowser(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{ChromeVersion: "120.0.0.0"}}
	fp := h.generateFingerprint(fingerprintRequest{OS: "windows", Browser: "chrome"})
	if fp.UserAgent == "" {
		t.Error("expected non-empty user agent")
	}
}

func TestGetStealthRecommendations_AllEnabled(t *testing.T) {
	h := Handlers{}
	features := map[string]bool{
		"user_agent_override":   true,
		"languages_spoofed":     true,
		"webrtc_leak_prevented": true,
		"timezone_spoofed":      true,
		"canvas_noise":          true,
		"font_spoofing":         true,
	}
	recs := h.getStealthRecommendations(features)
	if len(recs) != 1 || recs[0] != "Stealth mode is well configured" {
		t.Errorf("expected well-configured message, got %v", recs)
	}
}

func TestGetStealthRecommendations_NoneEnabled(t *testing.T) {
	h := Handlers{}
	features := map[string]bool{}
	recs := h.getStealthRecommendations(features)
	if len(recs) != 6 {
		t.Errorf("expected 6 recommendations, got %d: %v", len(recs), recs)
	}
}

func TestSendStealthResponse_HighScore(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	features := map[string]bool{
		"a": true, "b": true, "c": true, "d": true, "e": true,
	}
	w := httptest.NewRecorder()
	h.sendStealthResponse(w, features, "TestAgent/1.0")

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !searchString(body, `"level":"high"`) {
		t.Errorf("expected high level for 100%% score, got: %s", body)
	}
}

func TestStaticStealthFeatures_HasEntries(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{}}
	features := h.staticStealthFeatures()
	if len(features) == 0 {
		t.Error("expected non-empty stealth features map")
	}
}

func TestStealthScript_Content(t *testing.T) {
	if assets.StealthScript == "" {
		t.Fatal("StealthScript is empty")
	}
	if !strings.Contains(assets.StealthScript, "navigator") || !strings.Contains(assets.StealthScript, "webdriver") {
		t.Error("stealth script missing webdriver protection")
	}
}

func TestGetStealthRecommendations(t *testing.T) {
	h := Handlers{}
	features := map[string]bool{
		"user_agent_override": true,
	}
	recs := h.getStealthRecommendations(features)
	found := false
	for _, r := range recs {
		if strings.Contains(r, "Accept-Language") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected recommendation for Accept-Language spoofing")
	}
}

func TestGenerateFingerprint_Config(t *testing.T) {
	cfg := &config.RuntimeConfig{ChromeVersion: "120.0.0.0"}
	h := Handlers{Config: cfg}

	fp := h.generateFingerprint(fingerprintRequest{OS: "windows", Browser: "chrome"})
	if !strings.Contains(fp.UserAgent, "120.0.0.0") {
		t.Errorf("expected User-Agent to contain Chrome version 120.0.0.0, got %q", fp.UserAgent)
	}
}

func TestStealthScript_Populated(t *testing.T) {
	b := bridge.New(context.Background(), context.Background(), &config.RuntimeConfig{})
	b.StealthScript = assets.StealthScript

	if b.StealthScript == "" {
		t.Error("expected stealth script to be populated")
	}
}
