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

func TestGenerateFingerprint_Config(t *testing.T) {
	cfg := &config.RuntimeConfig{ChromeVersion: "120.0.0.0"}
	h := Handlers{Config: cfg}

	fp := h.generateFingerprint(fingerprintRequest{OS: "windows", Browser: "chrome"})
	if !strings.Contains(fp.UserAgent, "120.0.0.0") {
		t.Errorf("expected User-Agent to contain Chrome version 120.0.0.0, got %q", fp.UserAgent)
	}
}

func TestStealthScript_Content(t *testing.T) {
	if assets.StealthScript == "" {
		t.Fatal("StealthScript is empty")
	}
	if !strings.Contains(assets.StealthScript, "navigator") || !strings.Contains(assets.StealthScript, "webdriver") {
		t.Error("stealth script missing webdriver protection")
	}
	if !strings.Contains(assets.StealthScript, "new Proxy") || !strings.Contains(assets.StealthScript, "Object.defineProperty(window, 'navigator'") {
		t.Error("stealth script missing navigator proxy protection")
	}
}

func TestStealthScript_Populated(t *testing.T) {
	b := bridge.New(context.Background(), context.Background(), &config.RuntimeConfig{})
	b.StealthScript = assets.StealthScript

	if b.StealthScript == "" {
		t.Error("expected stealth script to be populated")
	}
}
