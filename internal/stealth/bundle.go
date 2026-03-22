package stealth

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/config"
)

type Level string

const (
	LevelLight  Level = "light"
	LevelMedium Level = "medium"
	LevelFull   Level = "full"
)

type LaunchMode string

const (
	LaunchModeUninitialized  LaunchMode = "uninitialized"
	LaunchModeAllocator      LaunchMode = "allocator"
	LaunchModeDirectFallback LaunchMode = "direct_fallback"
	LaunchModeAttached       LaunchMode = "attached"
)

type WebdriverMode string

const (
	WebdriverModeJSProxy WebdriverMode = "js_proxy"
)

type Bundle struct {
	Level        Level           `json:"level"`
	Seed         int64           `json:"seed"`
	Script       string          `json:"-"`
	ScriptHash   string          `json:"scriptHash"`
	PatchIDs     []string        `json:"patchIds"`
	Capabilities map[string]bool `json:"capabilities"`
	Tradeoffs    []string        `json:"tradeoffs,omitempty"`
	Webdriver    WebdriverMode   `json:"webdriverMode"`
}

type Status struct {
	Level         Level           `json:"level"`
	Headless      bool            `json:"headless"`
	LaunchMode    LaunchMode      `json:"launchMode"`
	ScriptHash    string          `json:"scriptHash"`
	WebdriverMode WebdriverMode   `json:"webdriverMode"`
	PatchIDs      []string        `json:"patchIds"`
	Flags         map[string]bool `json:"flags"`
	Capabilities  map[string]bool `json:"capabilities"`
	Tradeoffs     []string        `json:"tradeoffs,omitempty"`
	TabOverrides  map[string]bool `json:"tabOverrides"`
}

func NewBundle(cfg *config.RuntimeConfig, seed int64) *Bundle {
	levelName := ""
	if cfg != nil {
		levelName = cfg.StealthLevel
	}
	level := NormalizeLevel(levelName)
	script := fmt.Sprintf(
		"var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n%s\n%s",
		seed,
		level,
		assets.StealthScript,
		PopupGuardInitScript,
	)

	return &Bundle{
		Level:        level,
		Seed:         seed,
		Script:       script,
		ScriptHash:   hashScript(script),
		PatchIDs:     patchIDsForLevel(level),
		Capabilities: capabilityMap(level),
		Tradeoffs:    tradeoffs(level),
		Webdriver:    WebdriverModeJSProxy,
	}
}

func NormalizeLevel(level string) Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case string(LevelMedium):
		return LevelMedium
	case string(LevelFull):
		return LevelFull
	default:
		return LevelLight
	}
}

func StatusFromBundle(bundle *Bundle, cfg *config.RuntimeConfig, launchMode LaunchMode) *Status {
	if bundle == nil {
		return nil
	}

	status := &Status{
		Level:         bundle.Level,
		Headless:      cfg != nil && cfg.Headless,
		LaunchMode:    launchMode,
		ScriptHash:    bundle.ScriptHash,
		WebdriverMode: bundle.Webdriver,
		PatchIDs:      append([]string(nil), bundle.PatchIDs...),
		Flags:         flagStatus(cfg),
		Capabilities:  cloneBoolMap(bundle.Capabilities),
		Tradeoffs:     append([]string(nil), bundle.Tradeoffs...),
		TabOverrides: map[string]bool{
			"fingerprintRotateActive": false,
		},
	}
	if status.LaunchMode == "" {
		status.LaunchMode = LaunchModeUninitialized
	}
	return status
}

func hashScript(script string) string {
	sum := sha256.Sum256([]byte(script))
	return fmt.Sprintf("sha256:%x", sum)
}

func flagStatus(cfg *config.RuntimeConfig) map[string]bool {
	extraFlags := ""
	headless := false
	if cfg != nil {
		extraFlags = cfg.ChromeExtraFlags
		headless = cfg.Headless
	}

	return map[string]bool{
		"automationControlledDisabled": true,
		"enableAutomationFalse":        true,
		"headlessNew":                  headless,
		"swiftshader":                  headless,
		"downlinkMaxFlag":              hasFlag(extraFlags, "--enable-network-information-downlink-max"),
		"testTypeGPU":                  hasFlag(extraFlags, "--test-type=gpu"),
		"disableInfobars":              hasFlag(extraFlags, "--disable-infobars"),
		"disableDesktopNotifications":  hasFlag(extraFlags, "--disable-desktop-notifications"),
		"disableWindowActivation":      hasFlag(extraFlags, "--disable-window-activation"),
		"silentDebuggerExtensionAPI":   hasFlag(extraFlags, "--silent-debugger-extension-api"),
	}
}

func hasFlag(args string, want string) bool {
	for _, field := range strings.Fields(args) {
		if field == want {
			return true
		}
	}
	return false
}

func capabilityMap(level Level) map[string]bool {
	caps := map[string]bool{
		"webdriverNotTrue":            true,
		"webdriverNativeStrategy":     false,
		"batteryAPIBaseline":          true,
		"pluginArray":                 true,
		"userAgentData":               false,
		"chromeRuntimeConnect":        false,
		"chromeRuntimeSendMessage":    false,
		"chromeApp":                   false,
		"videoCodecs":                 false,
		"maxTouchPoints":              false,
		"iframeIsolation":             false,
		"errorStackSanitized":         false,
		"functionToStringMasked":      false,
		"downlinkMax":                 false,
		"webglSpoofing":               false,
		"canvasNoise":                 false,
		"systemColorFix":              false,
		"transparentPixelCanvasNoise": false,
		"audioNoise":                  false,
		"webrtcMitigation":            false,
	}

	if level == LevelMedium || level == LevelFull {
		caps["userAgentData"] = true
		caps["chromeRuntimeConnect"] = true
		caps["chromeRuntimeSendMessage"] = true
		caps["chromeApp"] = true
		caps["videoCodecs"] = true
		caps["maxTouchPoints"] = true
	}
	if level == LevelFull {
		caps["webglSpoofing"] = true
		caps["canvasNoise"] = true
		caps["audioNoise"] = true
		caps["webrtcMitigation"] = true
	}

	return caps
}

func patchIDsForLevel(level Level) []string {
	patches := []string{
		"marker-cleanup",
		"webdriver-proxy",
		"plugins",
		"languages",
		"platform",
		"hardware",
		"permissions",
		"screen",
		"battery",
	}

	if level == LevelMedium || level == LevelFull {
		patches = append(patches,
			"user-agent-data",
			"chrome-runtime",
			"chrome-app",
			"codecs",
			"touch-points",
			"prepare-stack-trace-lock",
		)
	}
	if level == LevelFull {
		patches = append(patches,
			"webgl",
			"canvas-noise",
			"font-noise",
			"audio-noise",
			"webrtc-relay",
		)
	}

	return patches
}

func tradeoffs(level Level) []string {
	switch level {
	case LevelMedium:
		return []string{
			"non-default-risk-mode",
			"error-monitoring-risk",
			"api-realism-risk",
		}
	case LevelFull:
		return []string{
			"non-default-risk-mode",
			"error-monitoring-risk",
			"api-realism-risk",
			"graphics-and-media-breakage-risk",
			"webrtc-behavior-risk",
		}
	default:
		return nil
	}
}

func cloneBoolMap(src map[string]bool) map[string]bool {
	if src == nil {
		return nil
	}
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
