package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/builtin"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/geo"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

var (
	runtimeGOOS              = goruntime.GOOS
	osGeteuid                = os.Geteuid
	containerMarkerPath      = "/.dockerenv"
	geoProviderForConfigFunc = geoProviderForConfig
)

const launchGeoLookupTimeout = 3 * time.Second

type launchGeoAlignment struct {
	info  geo.Info
	flags []string
	env   []string
}

type Hooks struct {
	SetHumanRandSeed        func(int64)
	IsProfileLockError      func(string) bool
	ClearStaleProfileLocks  func(profileDir, errMsg string) (bool, error)
	ConfigureBrowserProcess func(*exec.Cmd)
	// QuarantineCorruptedProfile moves profileDir aside and recreates an
	// empty dir at the same path. Used to recover from silent CDP attach
	// failures (observed with CloakBrowser when the profile dir holds
	// state it cannot ingest). Returns the quarantine path on success.
	QuarantineCorruptedProfile func(profileDir string) (string, error)
}

// InitBrowser initializes a browser for a Bridge instance.
//
// When cfg.CDPAttachURL is set, this skips launching a browser process and
// connects to the externally-managed browser at that browser-level CDP
// WebSocket URL. The returned cancel funcs only release the chromedp
// allocator + browser context; the external browser process is left alive.
func InitBrowser(cfg *config.RuntimeConfig, bundle *stealth.Bundle, hooks Hooks) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	if cfg != nil && strings.TrimSpace(cfg.CDPAttachURL) != "" {
		return initBrowserFromExistingCDP(cfg, bundle)
	}

	targetBinary := runtimekit.FindBrowserBinary(config.NormalizeBrowser(cfg.DefaultBrowser))
	if strings.TrimSpace(cfg.BrowserBinary) != "" {
		targetBinary = strings.TrimSpace(cfg.BrowserBinary)
	}
	slog.Info("starting browser initialization", "headless", cfg.Headless, "profile", cfg.ProfileDir, "binary", targetBinary)
	browserID := config.NormalizeBrowser(cfg.DefaultBrowser)
	if b, ok := browsers.Get(browserID); ok {
		tcfg := browsers.TargetConfig{
			Provider: browserID,
			Binary:   targetBinary,
		}
		if err := b.ValidateTarget(tcfg); err != nil {
			return nil, nil, nil, nil, stealth.LaunchModeUninitialized, missingBrowserBinaryError(cfg)
		}
	}

	bundle = ensureStealthBundle(cfg, bundle)
	geoAlignment, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to resolve proxy geo alignment: %w", err)
	}
	allocCtx, allocCancel, opts, debugPort, err := setupAllocator(cfg, bundle, hooks, geoAlignment)
	if err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, err
	}
	browserCtx, browserCancel, launchMode, err := startBrowser(allocCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment)
	if err != nil {
		allocCancel()
		slog.Error("browser initialization failed", "headless", cfg.Headless, "error", err.Error())
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to start browser: %w", err)
	}

	if ProxyAuthEnabled(cfg.Proxy) {
		if err := EnableProxyAuth(browserCtx, cfg.Proxy, nil); err != nil {
			browserCancel()
			allocCancel()
			return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to enable proxy auth: %w", err)
		}
		slog.Info("proxy authentication enabled via CDP", "proxy", cfg.Proxy.Redacted())
	}

	slog.Info("browser initialized successfully", "headless", cfg.Headless, "profile", cfg.ProfileDir)
	return allocCtx, allocCancel, browserCtx, browserCancel, launchMode, nil
}

// FindBrowserBinary exposes the launch-time browser discovery used by runtime
// initialization so diagnostics can report against the same search path.
func FindBrowserBinary() string {
	return runtimekit.FindBrowserBinary("chrome")
}

type providerLaunchPlan struct {
	browser browsers.Browser
	args    []string
	env     []string
	binary  string
}

func providerLaunchConfig(cfg *config.RuntimeConfig, binary string, debugPort int) browsers.LaunchConfig {
	return runtimekit.LaunchConfigFromRuntime(cfg, binary, debugPort, launchNeedsNoSandbox())
}

func resolveProviderLaunchPlan(cfg *config.RuntimeConfig, launchCfg browsers.LaunchConfig) (providerLaunchPlan, error) {
	plan, err := runtimekit.ResolveProviderLaunchPlan(cfg, launchCfg)
	if err != nil {
		return providerLaunchPlan{}, fmt.Errorf("%s launch args: %w", config.NormalizeBrowser(cfg.DefaultBrowser), err)
	}
	return providerLaunchPlan{
		browser: plan.Browser,
		args:    plan.Args,
		env:     plan.Env,
		binary:  plan.Binary,
	}, nil
}

func ensureStealthBundle(cfg *config.RuntimeConfig, bundle *stealth.Bundle) *stealth.Bundle {
	if bundle != nil {
		return bundle
	}
	return stealth.NewBundle(cfg, cryptoRandSeed())
}

func setupAllocator(cfg *config.RuntimeConfig, bundle *stealth.Bundle, hooks Hooks, geoAlignment launchGeoAlignment) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption, int, error) {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	}
	debugPort := 0
	if cfg.BrowserDebugPort > 0 {
		debugPort = cfg.BrowserDebugPort
	} else if port, err := findFreePort(cfg.InstancePortStart, cfg.InstancePortEnd); err == nil {
		debugPort = port
	}
	binary := strings.TrimSpace(cfg.BrowserBinary)
	if binary == "" {
		binary = runtimekit.FindBrowserBinary(config.NormalizeBrowser(cfg.DefaultBrowser))
	}
	launchCfg := runtimekit.LaunchConfigFromRuntime(cfg, binary, debugPort, launchNeedsNoSandbox())
	// setupAllocator appends user extra flags itself (below), where the
	// DisableInProcessGPU kill switch can strip them; the plan must not
	// re-inject a second, unstrippable copy.
	launchCfg.ExtraFlags = nil
	plan, err := resolveProviderLaunchPlan(cfg, launchCfg)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	opts = appendExecAllocatorFlags(opts, plan.args)
	opts = appendExecAllocatorFlags(opts, bundle.Launch.Args)
	proxyFlags, err := config.BrowserProxyFlags(cfg.Proxy)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	opts = appendExecAllocatorFlags(opts, proxyFlags)
	opts = appendExecAllocatorFlags(opts, geoAlignment.flags)

	if plan.binary != "" {
		opts = append(opts, chromedp.ExecPath(plan.binary))
	}

	if cfg.Headless {
		opts = append(opts, chromedp.Flag("headless", "new"))
		opts = append(opts, chromedp.Flag("hide-scrollbars", true))
		opts = append(opts, chromedp.Flag("mute-audio", true))
		opts = append(opts, chromedp.Flag("disable-vulkan", true))
		// Use swiftshader (software GPU) for compositing under --headless=new.
		// We deliberately do NOT pass --disable-gpu here: in new-headless
		// mode Page.captureScreenshot routes through the compositor, which
		// needs a GPU backend — disabling the GPU process leaves the
		// compositor with no backend and screenshot calls hang past the
		// action timeout.
		opts = append(opts, chromedp.Flag("use-angle", "swiftshader"))
		// Chromium 121+ requires this opt-in to actually load the
		// swiftshader backend; without it, --use-angle=swiftshader is
		// silently ignored and the compositor has no backend, which
		// manifests as Page.captureScreenshot/printToPDF hanging.
		opts = append(opts, chromedp.Flag("enable-unsafe-swiftshader", true))
		// --in-process-gpu used to be enabled here as a perf optimization
		// (saves one OS process and ~50-150MB per instance). Chrome stable
		// patch updates have repeatedly regressed it for the headless=new
		// + swiftshader combo: a GPU code crash takes the browser with it
		// ~500ms after init, surfacing as `context canceled` on the first
		// CreateTab. We now leave it off by default. Users who know their
		// Chrome build is healthy and want the memory savings can opt in
		// via `browser.extraFlags = "--in-process-gpu"`. DisableInProcessGPU
		// is still honored as a kill switch by the crash-recovery path.
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	if validPaths := existingExtensionPaths(cfg.ExtensionPaths); len(validPaths) > 0 {
		joined := strings.Join(validPaths, ",")
		opts = append(opts, chromedp.Flag("disable-extensions", false))
		opts = append(opts, chromedp.Flag("load-extension", joined))
		opts = append(opts, chromedp.Flag("disable-extensions-except", joined))
		slog.Info("loading extensions", "paths", joined)
	} else {
		opts = append(opts, chromedp.Flag("disable-extensions", true))
	}

	if cfg.ProfileDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.ProfileDir))
	}

	w, h := randomWindowSize()
	opts = append(opts, chromedp.WindowSize(w, h))

	if cfg.Timezone != "" {
		opts = append(opts, chromedp.Flag("tz", cfg.Timezone))
	}

	extraFlags := config.AllowedBrowserExtraFlags(cfg.BrowserExtraFlags)
	if cfg.DisableInProcessGPU {
		// Kill switch from the crash-recovery path: strip a user-supplied
		// --in-process-gpu so a crash loop can't repeat after retry.
		extraFlags = stripInProcessGPUFlag(extraFlags)
	}
	opts = appendExecAllocatorFlags(opts, extraFlags)
	for _, flag := range appendBrowserCompatibilityFlags(nil) {
		opts = appendExecAllocatorFlag(opts, flag)
	}

	opts = append(opts, chromedp.CombinedOutput(newPrefixedLogWriter(os.Stdout, "browser")))
	opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		if len(plan.env) > 0 || len(geoAlignment.env) > 0 {
			if cmd.Env == nil {
				cmd.Env = os.Environ()
			}
			if len(plan.env) > 0 {
				cmd.Env = mergeGeoEnv(cmd.Env, plan.env)
			}
			cmd.Env = mergeGeoEnv(cmd.Env, geoAlignment.env)
		}
		if hooks.ConfigureBrowserProcess != nil {
			hooks.ConfigureBrowserProcess(cmd)
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	return ctx, cancel, opts, debugPort, nil
}

func applyStartupStealth(ctx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, script string) error {
	if !config.PinchTabStealthDefaultsDisabled(cfg) {
		ua := ""
		if bundle != nil {
			ua = bundle.LaunchUserAgent()
		}
		if err := stealth.ApplyTargetEmulation(ctx, cfg, ua); err != nil {
			return err
		}
	}
	if strings.TrimSpace(script) == "" {
		return nil
	}
	return injectedScript(ctx, script)
}

func missingBrowserBinaryError(cfg *config.RuntimeConfig) error {
	browserID := config.NormalizeBrowser(cfg.DefaultBrowser)
	if b, ok := browsers.Get(browserID); ok {
		name := b.DisplayName()
		return fmt.Errorf("%s binary not found: set browser.binary to the %s binary path", strings.ToLower(name), name)
	}
	return fmt.Errorf("browser binary not found: set browser.binary in config")
}

func injectedScript(ctx context.Context, script string) error {
	return chromedp.FromContext(ctx).Target.Execute(ctx,
		"Page.addScriptToEvaluateOnNewDocument",
		map[string]interface{}{
			"source": script,
		}, nil)
}

func geoProviderForConfig(cfg *config.RuntimeConfig) geo.Provider {
	if cfg == nil || cfg.Proxy.Geo == nil || cfg.Proxy.Geo.IsZero() {
		return geo.Noop{}
	}
	return geo.Static{Info: cfg.Proxy.GeoInfo()}
}

func resolveLaunchGeoAlignment(parent context.Context, cfg *config.RuntimeConfig) (launchGeoAlignment, error) {
	if cfg == nil || cfg.Proxy.IsZero() || cfg.Proxy.Geo == nil || cfg.Proxy.Geo.IsZero() {
		return launchGeoAlignment{}, nil
	}
	if !config.CloakBrowserActive(cfg) {
		return launchGeoAlignment{}, nil
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, launchGeoLookupTimeout)
	defer cancel()

	info, err := geoProviderForConfigFunc(cfg).Lookup(ctx, "")
	if err != nil {
		return launchGeoAlignment{}, err
	}
	flags, env := applyGeoAlignment(cfg.DefaultBrowser, info, cfg.Cloak)
	return launchGeoAlignment{
		info:  info,
		flags: flags,
		env:   env,
	}, nil
}

// mergeGeoEnv overlays additions over base by key; base is not mutated.
func mergeGeoEnv(base, additions []string) []string {
	if len(additions) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(additions))
	overrideKeys := make(map[string]struct{}, len(additions))
	for _, kv := range additions {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			overrideKeys[kv[:eq]] = struct{}{}
		}
	}
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 {
			if _, replace := overrideKeys[kv[:eq]]; replace {
				continue
			}
		}
		out = append(out, kv)
	}
	out = append(out, additions...)
	return out
}
