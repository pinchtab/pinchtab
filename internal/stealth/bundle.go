package stealth

import (
	"crypto/sha256"
	"encoding/json"
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
	WebdriverModeNativeBaseline WebdriverMode = "native_baseline"
)

type Bundle struct {
	Level        Level           `json:"level"`
	Seed         int64           `json:"seed"`
	Script       string          `json:"-"`
	ScriptHash   string          `json:"scriptHash"`
	Launch       LaunchContract  `json:"-"`
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
	UserAgent     string          `json:"userAgent,omitempty"`
	WebdriverMode WebdriverMode   `json:"webdriverMode"`
	PatchIDs      []string        `json:"patchIds"`
	Flags         map[string]bool `json:"flags"`
	Capabilities  map[string]bool `json:"capabilities"`
	Tradeoffs     []string        `json:"tradeoffs,omitempty"`
	TabOverrides  map[string]bool `json:"tabOverrides"`
}

func NewBundle(cfg *config.RuntimeConfig, seed int64) *Bundle {
	levelName := ""
	headless := false
	if cfg != nil {
		levelName = cfg.StealthLevel
		headless = cfg.Headless
	}
	level := NormalizeLevel(levelName)
	personaJSON := "{}"
	if cfg != nil {
		if encoded, err := json.Marshal(BuildPersona(cfg.UserAgent, cfg.ChromeVersion)); err == nil {
			personaJSON = string(encoded)
		}
	}
	script := renderBundleScript(level, personaJSON, seed, headless)

	return &Bundle{
		Level:  level,
		Seed:   seed,
		Script: script,
		// Keep the status hash stable across identical configs even when the
		// per-process seed changes. The seed is runtime entropy, not contract.
		ScriptHash:   hashScript(renderBundleScript(level, personaJSON, 0, headless)),
		Launch:       BuildLaunchContract(cfg, level),
		PatchIDs:     patchIDsForLevel(level, headless),
		Capabilities: capabilityMap(level, headless),
		Tradeoffs:    tradeoffs(level),
		Webdriver:    WebdriverModeNativeBaseline,
	}
}

func renderBundleScript(level Level, personaJSON string, seed int64, headless bool) string {
	return fmt.Sprintf(
		"var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\nvar __pinchtab_headless = %t;\nvar __pinchtab_profile = %s;\n%s\n%s",
		seed,
		level,
		headless,
		personaJSON,
		assets.StealthScript,
		PopupGuardInitScript,
	)
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
		UserAgent:     bundle.LaunchUserAgent(),
		WebdriverMode: bundle.Webdriver,
		PatchIDs:      append([]string(nil), bundle.PatchIDs...),
		Flags:         statusFlags(bundle, cfg),
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

func statusFlags(bundle *Bundle, cfg *config.RuntimeConfig) map[string]bool {
	var extraFlags []string
	headless := false
	if cfg != nil {
		extraFlags = config.AllowedChromeExtraFlags(cfg.ChromeExtraFlags)
		headless = cfg.Headless
	}

	flags := map[string]bool{}
	if bundle != nil {
		flags = cloneBoolMap(bundle.Launch.Flags)
	}
	flags["headlessNew"] = headless
	flags["swiftshader"] = headless
	flags["testTypeGPU"] = hasFlag(extraFlags, "--test-type=gpu")
	flags["disableInfobars"] = hasFlag(extraFlags, "--disable-infobars")
	flags["disableDesktopNotifications"] = hasFlag(extraFlags, "--disable-desktop-notifications")
	flags["disableWindowActivation"] = hasFlag(extraFlags, "--disable-window-activation")
	flags["silentDebuggerExtensionAPI"] = hasFlag(extraFlags, "--silent-debugger-extension-api")
	return flags
}

func hasFlag(args []string, want string) bool {
	for _, field := range args {
		if field == want {
			return true
		}
	}
	return false
}

func capabilityMap(level Level, headless bool) map[string]bool {
	caps := map[string]bool{
		"webdriverNotTrue":                  true,
		"webdriverNativeStrategy":           true,
		"batteryAPIBaseline":                true,
		"pluginArray":                       true,
		"workerHardwareConsistency":         true,
		"workerUserAgentConsistency":        true,
		"serviceWorkerHardwareConsistency":  true,
		"serviceWorkerUserAgentConsistency": true,
		"userAgentData":                     false,
		"chromeRuntimeConnect":              false,
		"chromeRuntimeSendMessage":          false,
		"chromeApp":                         false,
		"videoCodecs":                       false,
		"maxTouchPoints":                    false,
		"iframeIsolation":                   false,
		"errorStackSanitized":               false,
		"functionToStringMasked":            false,
		"functionToStringNative":            true,
		"intlLocaleCoherent":                true,
		"errorPrepareStackTraceNative":      true,
		"navigatorOverridesAbsent":          true,
		"userAgentDataVersionCoherent":      true,
		"downlinkMax":                       true,
		"webglSpoofing":                     false,
		"canvasNoise":                       false,
		"systemColorFix":                    false,
		"transparentPixelCanvasNoise":       false,
		"audioNoise":                        false,
		"webrtcMitigation":                  false,
	}

	if level == LevelMedium || level == LevelFull {
		caps["userAgentData"] = true
		caps["maxTouchPoints"] = true
	}
	if level == LevelMedium {
		caps["iframeIsolation"] = true
		caps["chromeRuntimeConnect"] = true
		caps["chromeRuntimeSendMessage"] = true
	}
	if level == LevelFull && headless {
		caps["webglSpoofing"] = true
	}

	return caps
}

func patchIDsForLevel(level Level, headless bool) []string {
	patches := []string{
		"marker-cleanup",
		"webdriver-native-baseline",
		"plugins",
		"languages",
		"platform",
		"downlink-max",
		"permissions",
		"battery",
	}
	if headless {
		patches = append(patches, "screen")
	}

	if level == LevelMedium || level == LevelFull {
		patches = append(patches,
			"user-agent-data",
			"touch-points",
		)
	}
	if level == LevelMedium {
		patches = append(patches, "chrome-runtime", "iframe-isolation")
	}
	if level == LevelFull {
		if headless {
			patches = append(patches, "screen-position")
		}
		if headless {
			patches = append(patches, "webgl")
		}
	}

	return patches
}

func tradeoffs(level Level) []string {
	switch level {
	case LevelMedium:
		return []string{
			"non-default-risk-mode",
			"api-realism-risk",
		}
	case LevelFull:
		return []string{
			"non-default-risk-mode",
			"api-realism-risk",
			"graphics-and-media-breakage-risk",
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

func (b *Bundle) LaunchUserAgent() string {
	if b == nil {
		return ""
	}
	for _, arg := range b.Launch.Args {
		if strings.HasPrefix(arg, "--user-agent=") {
			return strings.TrimPrefix(arg, "--user-agent=")
		}
	}
	return ""
}
