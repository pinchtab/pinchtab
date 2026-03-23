package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

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

func (h *Handlers) HandleFingerprintRotate(w http.ResponseWriter, r *http.Request) {
	var req fingerprintRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	fp := h.generateFingerprint(req)

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
			if fp.Language != "" {
				if err := emulation.SetLocaleOverride().WithLocale(fp.Language).Do(ctx); err != nil {
					return fmt.Errorf("setLocaleOverride: %w", err)
				}
			}
			if timezoneID := timezoneIDFromOffset(fp.TimezoneOffset); timezoneID != "" {
				if err := emulation.SetTimezoneOverride(timezoneID).Do(ctx); err != nil {
					return fmt.Errorf("setTimezoneOverride: %w", err)
				}
			}
			if fp.ScreenWidth > 0 && fp.ScreenHeight > 0 {
				if err := emulation.SetDeviceMetricsOverride(int64(fp.ScreenWidth), int64(fp.ScreenHeight), 1, false).
					WithScreenWidth(int64(fp.ScreenWidth)).
					WithScreenHeight(int64(fp.ScreenHeight)).
					Do(ctx); err != nil {
					return fmt.Errorf("setDeviceMetricsOverride: %w", err)
				}
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if fp.Platform == "" {
				return nil
			}
			overlayScript := fingerprintRotatePlatformOverlayScript(fp.Platform)
			if _, err := page.AddScriptToEvaluateOnNewDocument(overlayScript).Do(ctx); err != nil {
				return fmt.Errorf("add platform overlay: %w", err)
			}
			return nil
		}),
		chromedp.Evaluate(fingerprintRotatePlatformOverlayScript(fp.Platform), nil),
	); err != nil {
		httpx.Error(w, 500, fmt.Errorf("CDP UA override: %w", err))
		return
	}

	if tracker, ok := h.Bridge.(interface{ SetFingerprintRotateActive(string, bool) }); ok {
		tracker.SetFingerprintRotateActive(resolvedTabID, true)
	}

	httpx.JSON(w, 200, map[string]any{
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

func (h *Handlers) generateFingerprint(req fingerprintRequest) fingerprint {
	fp := fingerprint{}

	osConfigs := map[string]map[string]fingerprint{
		"windows": {
			"chrome": {
				UserAgent: fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", h.Config.ChromeVersion),
				Platform:  "Win32",
				Vendor:    "Google Inc.",
			},
			"edge": {
				UserAgent: fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36 Edg/%s", h.Config.ChromeVersion, h.Config.ChromeVersion),
				Platform:  "Win32",
				Vendor:    "Google Inc.",
			},
		},
		"mac": {
			"chrome": {
				UserAgent: fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", h.Config.ChromeVersion),
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

func timezoneIDFromOffset(offset int) string {
	switch offset {
	case -480:
		return "America/Los_Angeles"
	case -420:
		return "America/Denver"
	case -360:
		return "America/Chicago"
	case -300:
		return "America/New_York"
	case -240:
		return "America/Halifax"
	case 0:
		return "UTC"
	case 60:
		return "Europe/Berlin"
	case 120:
		return "Europe/Helsinki"
	case 330:
		return "Asia/Kolkata"
	case 480:
		return "Asia/Shanghai"
	case 540:
		return "Asia/Tokyo"
	default:
		return ""
	}
}

func fingerprintRotatePlatformOverlayScript(platform string) string {
	return fmt.Sprintf(`(() => {
  try {
    const nav = navigator;
    const proto = Object.getPrototypeOf(nav) || Navigator.prototype;
    Object.defineProperty(proto, 'platform', {
      get: () => %q,
      configurable: true,
      enumerable: true
    });
  } catch (e) {}
})()`, platform)
}
