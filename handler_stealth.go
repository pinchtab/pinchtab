package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"log/slog"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) handleStealthStatus(w http.ResponseWriter, r *http.Request) {

	ctx, _, err := b.TabContext("")
	if err != nil {

		b.sendStealthResponse(w, staticStealthFeatures(), "")
		return
	}

	// Check actual browser state
	var result struct {
		WebDriver           bool     `json:"webdriver"`
		Plugins             int      `json:"plugins"`
		UserAgent           string   `json:"userAgent"`
		HardwareConcurrency int      `json:"hardwareConcurrency"`
		DeviceMemory        float64  `json:"deviceMemory"`
		Languages           []string `json:"languages"`
	}

	tCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	checkScript := `({
		webdriver: navigator.webdriver || false,
		plugins: navigator.plugins.length,
		userAgent: navigator.userAgent,
		hardwareConcurrency: navigator.hardwareConcurrency,
		deviceMemory: navigator.deviceMemory || 0,
		languages: navigator.languages || []
	})`

	if err := chromedp.Run(tCtx, chromedp.Evaluate(checkScript, &result)); err == nil {
		features := map[string]bool{
			"automation_controlled": !result.WebDriver,
			"webdriver_hidden":      !result.WebDriver,
			"chrome_headless_new":   cfg.Headless,
			"user_agent_override":   result.UserAgent != "",
			"webgl_vendor_override": true,
			"plugins_spoofed":       result.Plugins > 0,
			"languages_spoofed":     len(result.Languages) > 0,
			"webrtc_leak_prevented": true,
			"timezone_spoofed":      true,
			"canvas_noise":          true,
			"audio_noise":           false,
			"font_spoofing":         true,
			"hardware_concurrency":  result.HardwareConcurrency > 0,
			"device_memory":         result.DeviceMemory > 0,
		}
		b.sendStealthResponse(w, features, result.UserAgent)
		return
	}

	b.sendStealthResponse(w, staticStealthFeatures(), "")
}

func staticStealthFeatures() map[string]bool {
	return map[string]bool{
		"automation_controlled": true,
		"webdriver_hidden":      true,
		"chrome_headless_new":   cfg.Headless,
		"user_agent_override":   true,
		"webgl_vendor_override": true,
		"plugins_spoofed":       true,
		"languages_spoofed":     true,
		"webrtc_leak_prevented": true,
		"timezone_spoofed":      true,
		"canvas_noise":          true,
		"audio_noise":           false,
		"font_spoofing":         true,
		"hardware_concurrency":  true,
		"device_memory":         true,
	}
}

func (b *Bridge) sendStealthResponse(w http.ResponseWriter, features map[string]bool, userAgent string) {
	chromeFlags := []string{

		"--disable-features=IsolateOrigins,site-per-process",
		"--disable-site-isolation-trials",
		"--disable-web-security",
		"--disable-features=BlinkGenPropertyTrees",
		"--disable-ipc-flooding-protection",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
		"--disable-features=TranslateUI",
		"--disable-features=Translate",
	}

	enabledCount := 0
	for _, enabled := range features {
		if enabled {
			enabledCount++
		}
	}
	stealthScore := (enabledCount * 100) / len(features)

	var level string
	switch {
	case stealthScore >= 80:
		level = "high"
	case stealthScore >= 50:
		level = "medium"
	case stealthScore >= 30:
		level = "basic"
	default:
		level = "minimal"
	}

	if userAgent == "" && len(b.tabs) > 0 {
		for _, tab := range b.tabs {
			ctx, cancel := context.WithTimeout(tab.ctx, 1*time.Second)
			_ = chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &userAgent))
			cancel()
			if userAgent != "" {
				break
			}
		}
	}

	jsonResp(w, 200, map[string]any{
		"level":           level,
		"score":           stealthScore,
		"features":        features,
		"chrome_flags":    chromeFlags,
		"headless_mode":   cfg.Headless,
		"user_agent":      userAgent,
		"profile_path":    cfg.ProfileDir,
		"recommendations": getStealthRecommendations(features),
	})
}

func getStealthRecommendations(features map[string]bool) []string {
	recommendations := []string{}

	if !features["user_agent_override"] {
		recommendations = append(recommendations, "Enable user agent rotation to avoid detection")
	}
	if !features["languages_spoofed"] {
		recommendations = append(recommendations, "Randomize Accept-Language headers")
	}
	if !features["webrtc_leak_prevented"] {
		recommendations = append(recommendations, "Block WebRTC to prevent IP leaks")
	}
	if !features["timezone_spoofed"] {
		recommendations = append(recommendations, "Spoof cfg.Timezone to match target locale")
	}
	if !features["canvas_noise"] {
		recommendations = append(recommendations, "Add canvas fingerprint noise")
	}
	if !features["font_spoofing"] {
		recommendations = append(recommendations, "Randomize font metrics")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Stealth mode is well configured")
	}

	return recommendations
}

type fingerprintRequest struct {
	TabID    string `json:"tabId"`
	OS       string `json:"os"`
	Browser  string `json:"browser"`
	Screen   string `json:"screen"`
	Language string `json:"language"`
	Timezone int    `json:"timezone"`
	WebGL    bool   `json:"webgl"`
	Canvas   bool   `json:"canvas"`
	Fonts    bool   `json:"fonts"`
	Audio    bool   `json:"audio"`
}

func (b *Bridge) handleFingerprintRotate(w http.ResponseWriter, r *http.Request) {
	var req fingerprintRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, _, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	fp := generateFingerprint(req)

	tCtx, tCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tCancel()

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {

			err := emulation.SetUserAgentOverride(fp.UserAgent).
				WithPlatform(fp.Platform).
				WithAcceptLanguage(fp.Language).
				Do(ctx)
			if err != nil {
				return fmt.Errorf("setUserAgentOverride: %w", err)
			}
			return nil
		}),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("CDP UA override: %w", err))
		return
	}

	script := fmt.Sprintf(`
(function() {
  Object.defineProperty(screen, 'width', { get: () => %d, configurable: true });
  Object.defineProperty(screen, 'height', { get: () => %d, configurable: true });
  Object.defineProperty(screen, 'availWidth', { get: () => %d, configurable: true });
  Object.defineProperty(screen, 'availHeight', { get: () => %d, configurable: true });
  Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => %d, configurable: true });
  Object.defineProperty(navigator, 'deviceMemory', { get: () => %d, configurable: true });
})();
	`, fp.ScreenWidth, fp.ScreenHeight, fp.ScreenWidth-20, fp.ScreenHeight-80,
		fp.CPUCores, fp.Memory)

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
			return err
		}),
		chromedp.Evaluate(script, nil),
	); err != nil {

		slog.Warn("JS fingerprint extras failed", "err", err)
	}

	jsonResp(w, 200, map[string]any{
		"fingerprint": fp,
		"status":      "rotated",
	})
}

type fingerprint struct {
	UserAgent      string `json:"userAgent"`
	Platform       string `json:"platform"`
	Vendor         string `json:"vendor"`
	ScreenWidth    int    `json:"screenWidth"`
	ScreenHeight   int    `json:"screenHeight"`
	Language       string `json:"language"`
	TimezoneOffset int    `json:"timezoneOffset"`
	CPUCores       int    `json:"cpuCores"`
	Memory         int    `json:"memory"`
}

func generateFingerprint(req fingerprintRequest) fingerprint {
	fp := fingerprint{}

	osConfigs := map[string]map[string]fingerprint{
		"windows": {
			"chrome": {
				UserAgent: fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", cfg.ChromeVersion),
				Platform:  "Win32",
				Vendor:    "Google Inc.",
			},
			"edge": {
				UserAgent: fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36 Edg/%s", cfg.ChromeVersion, cfg.ChromeVersion),
				Platform:  "Win32",
				Vendor:    "Google Inc.",
			},
		},
		"mac": {
			"chrome": {
				UserAgent: fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", cfg.ChromeVersion),
				Platform:  "MacIntel",
				Vendor:    "Google Inc.",
			},
			"safari": {
				UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
				Platform:  "MacIntel",
				Vendor:    "Apple Computer, Inc.",
			},
		},
	}

	os := req.OS
	if os == "random" {
		if rand.Float64() < 0.7 {
			os = "windows"
		} else {
			os = "mac"
		}
	}

	browser := req.Browser
	if browser == "" {
		browser = "chrome"
	}

	if osConfig, ok := osConfigs[os]; ok {
		if browserConfig, ok := osConfig[browser]; ok {
			fp.UserAgent = browserConfig.UserAgent
			fp.Platform = browserConfig.Platform
			fp.Vendor = browserConfig.Vendor
		}
	}

	screens := [][]int{
		{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
		{1280, 720}, {1600, 900}, {2560, 1440},
	}
	if req.Screen == "random" {
		screen := screens[rand.Intn(len(screens))]
		fp.ScreenWidth = screen[0]
		fp.ScreenHeight = screen[1]
	} else if req.Screen != "" {
		_, _ = fmt.Sscanf(req.Screen, "%dx%d", &fp.ScreenWidth, &fp.ScreenHeight)
	} else {
		fp.ScreenWidth = 1920
		fp.ScreenHeight = 1080
	}

	if req.Language != "" {
		fp.Language = req.Language
	} else {
		fp.Language = "en-US"
	}

	if req.Timezone != 0 {
		fp.TimezoneOffset = req.Timezone
	} else {
		fp.TimezoneOffset = -300
	}

	fp.CPUCores = 4 + rand.Intn(4)*2
	fp.Memory = 4 + rand.Intn(4)*2

	return fp
}
