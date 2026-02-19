package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStealthJavaScript(t *testing.T) {

	content := stealthScript

	features := []string{
		"navigator, 'webdriver'",
		"sessionSeed",
		"seededRandom",
		"toDataURL",
		"measureText",
		"hardwareConcurrency",
		"deviceMemory",
		"__pinchtab_timezone",
	}

	for _, feature := range features {
		if !strings.Contains(content, feature) {
			t.Errorf("stealth script missing feature: %s", feature)
		}
	}

	count := strings.Count(content, "const sessionSeed")
	if count != 1 {
		t.Errorf("expected 1 sessionSeed definition, found %d", count)
	}
}

func TestGetStealthRecommendations(t *testing.T) {
	allFeatures := map[string]bool{
		"user_agent_override":   true,
		"languages_spoofed":     true,
		"webrtc_leak_prevented": true,
		"timezone_spoofed":      true,
		"canvas_noise":          true,
		"font_spoofing":         true,
		"audio_noise":           true,
	}

	tests := []struct {
		name     string
		features map[string]bool
		wantRecs int
	}{
		{
			name:     "all enabled",
			features: allFeatures,
			wantRecs: 1,
		},
		{
			name: "canvas disabled",
			features: map[string]bool{
				"user_agent_override":   true,
				"languages_spoofed":     true,
				"webrtc_leak_prevented": true,
				"timezone_spoofed":      true,
				"canvas_noise":          false,
				"font_spoofing":         true,
				"audio_noise":           true,
			},
			wantRecs: 1,
		},
		{
			name: "multiple disabled",
			features: map[string]bool{
				"user_agent_override":   false,
				"languages_spoofed":     false,
				"webrtc_leak_prevented": false,
				"timezone_spoofed":      true,
				"canvas_noise":          true,
				"font_spoofing":         true,
				"audio_noise":           true,
			},
			wantRecs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := getStealthRecommendations(tt.features)
			if len(recs) != tt.wantRecs {
				t.Errorf("expected %d recommendations, got %d: %v", tt.wantRecs, len(recs), recs)
			}
		})
	}
}

func TestGenerateFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		req      fingerprintRequest
		wantUA   string
		wantPlat string
	}{
		{
			name: "windows_chrome",
			req: fingerprintRequest{
				OS:      "windows",
				Browser: "chrome",
			},
			wantUA:   "Chrome/" + cfg.ChromeVersion,
			wantPlat: "Win32",
		},
		{
			name: "mac_chrome",
			req: fingerprintRequest{
				OS:      "mac",
				Browser: "chrome",
			},
			wantUA:   "Chrome/" + cfg.ChromeVersion,
			wantPlat: "MacIntel",
		},
		{
			name: "random_chrome",
			req: fingerprintRequest{
				OS:      "random",
				Browser: "chrome",
			},
			wantUA:   "Chrome/" + cfg.ChromeVersion,
			wantPlat: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := generateFingerprint(tt.req)

			if tt.wantUA != "" && !strings.Contains(fp.UserAgent, tt.wantUA) {
				t.Errorf("expected UA to contain %s, got %s", tt.wantUA, fp.UserAgent)
			}

			if tt.wantPlat != "" && fp.Platform != tt.wantPlat {
				t.Errorf("expected platform %s, got %s", tt.wantPlat, fp.Platform)
			}

			if fp.UserAgent == "" {
				t.Error("UserAgent is empty")
			}
			if fp.Platform == "" && tt.req.OS != "random" {
				t.Error("Platform is empty")
			}
		})
	}
}

func TestHandleStealthStatus(t *testing.T) {
	b := newTestBridgeWithTabs()

	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	b.handleStealthStatus(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Level       string          `json:"level"`
		Score       int             `json:"score"`
		Features    map[string]bool `json:"features"`
		ChromeFlags []string        `json:"chrome_flags"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Level == "" {
		t.Error("level is empty")
	}
	if resp.Score < 0 || resp.Score > 100 {
		t.Errorf("invalid score: %d", resp.Score)
	}
	if len(resp.Features) == 0 {
		t.Error("features is empty")
	}
	if len(resp.ChromeFlags) == 0 {
		t.Error("chrome_flags is empty")
	}

	enabled := 0
	for _, v := range resp.Features {
		if v {
			enabled++
		}
	}
	expectedScore := (enabled * 100) / len(resp.Features)
	if resp.Score != expectedScore {
		t.Errorf("score mismatch: got %d, expected %d", resp.Score, expectedScore)
	}
}
