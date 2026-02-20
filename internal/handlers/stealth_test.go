package handlers

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/config"
)

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
