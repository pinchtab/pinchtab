package runtimekit

import (
	"log/slog"
	"os"
	goruntime "runtime"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
)

type ProviderLaunchPlan struct {
	Browser browsers.Browser
	Args    []string
	Env     []string
	Binary  string
}

func RuntimeProxyConfig(cfg *config.RuntimeConfig) browsers.ProxyConfig {
	if cfg == nil {
		return browsers.ProxyConfig{}
	}
	out := browsers.ProxyConfig{
		Server:   cfg.Proxy.Server,
		Username: cfg.Proxy.Username,
		Password: cfg.Proxy.Password,
	}
	if len(cfg.Proxy.BypassList) > 0 {
		out.BypassList = append([]string(nil), cfg.Proxy.BypassList...)
	}
	if cfg.Proxy.Geo != nil {
		out.Geo = &browsers.GeoConfig{
			Timezone:   cfg.Proxy.Geo.Timezone,
			Locale:     cfg.Proxy.Geo.Locale,
			WebRTCIP:   cfg.Proxy.Geo.WebRTCIP,
			CountryISO: cfg.Proxy.Geo.CountryISO,
		}
	}
	return out
}

func LaunchConfigFromRuntime(cfg *config.RuntimeConfig, binary string, debugPort int, noSandbox bool) browsers.LaunchConfig {
	return browsers.LaunchConfig{
		Mode:           defaultBrowserToLaunchMode(cfg.DefaultBrowser),
		Binary:         binary,
		ProfileDir:     cfg.ProfileDir,
		Proxy:          RuntimeProxyConfig(cfg),
		ExtraFlags:     config.AllowedBrowserExtraFlags(cfg.BrowserExtraFlags),
		Headless:       cfg.Headless,
		Timezone:       cfg.Timezone,
		ExtensionPaths: cfg.ExtensionPaths,
		DebugPort:      debugPort,
		NoRestore:      cfg.NoRestore,
		UserAgent:      cfg.UserAgent,
		NoSandbox:      noSandbox,
		Cloak: browsers.CloakFingerprint{
			FingerprintSeed: cfg.Cloak.FingerprintSeed,
			Platform:        cfg.Cloak.Platform,
			Locale:          cfg.Cloak.Locale,
			Timezone:        cfg.Cloak.Timezone,
			WebRTCIP:        cfg.Cloak.WebRTCIP,
			FontsDir:        cfg.Cloak.FontsDir,
			StorageQuotaMB:  cfg.Cloak.StorageQuotaMB,
		},
	}
}

func ResolveProviderLaunchPlan(cfg *config.RuntimeConfig, launchCfg browsers.LaunchConfig) (ProviderLaunchPlan, error) {
	browserID := config.NormalizeBrowser(cfg.DefaultBrowser)
	browser := browsers.MustGet(browserID)
	if strings.TrimSpace(launchCfg.Binary) == "" {
		launchCfg.Binary = browser.DiscoverBinary().Found
	}
	args, env, err := browser.BuildLaunchArgs(launchCfg)
	if err != nil {
		return ProviderLaunchPlan{}, err
	}
	return ProviderLaunchPlan{
		Browser: browser,
		Args:    args,
		Env:     env,
		Binary:  strings.TrimSpace(launchCfg.Binary),
	}, nil
}

func FindBrowserBinary(browserID string) string {
	b, ok := browsers.Get(browserID)
	if !ok {
		return ""
	}
	return b.DiscoverBinary().Found
}

func BaseFlagArgs(browserID string, headless bool) []string {
	args, _, err := browsers.MustGet(browserID).BuildLaunchArgs(browsers.LaunchConfig{Headless: headless})
	if err != nil {
		slog.Warn("base flag args build failed", "browser", browserID, "err", err)
	}
	return args
}

// CloakBrowserFlagArgs extracts the --fingerprint* flags for the configured
// cloak settings. Only fingerprint flags survive the filter below, so the
// sandbox decision feeding BuildLaunchArgs never reaches the output.
func CloakBrowserFlagArgs(cfg *config.RuntimeConfig) []string {
	if cfg == nil || !config.IsCloakBrowser(cfg.DefaultBrowser) {
		return nil
	}
	args, _, err := browsers.MustGet(config.BrowserCloak).BuildLaunchArgs(
		LaunchConfigFromRuntime(cfg, "", 0, ChromeNeedsNoSandbox(goruntime.GOOS, os.Geteuid(), false)),
	)
	if err != nil {
		slog.Warn("cloak fingerprint flag build failed", "err", err)
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "--fingerprint") {
			out = append(out, arg)
		}
	}
	return out
}

func ChromeNeedsNoSandbox(goos string, euid int, inContainer bool) bool {
	if goos != "linux" {
		return false
	}
	if euid == 0 {
		return true
	}
	return inContainer
}

func defaultBrowserToLaunchMode(defaultBrowser string) browsers.LaunchMode {
	if config.NormalizeBrowser(defaultBrowser) == config.BrowserGhostChrome {
		return browsers.LaunchModeAuto
	}
	return browsers.LaunchModeChrome
}
