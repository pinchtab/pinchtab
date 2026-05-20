package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browserprobe"
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
	SetHumanRandSeed          func(int64)
	IsChromeProfileLockError  func(string) bool
	ClearStaleChromeProfile   func(profileDir, errMsg string) (bool, error)
	ConfigureChromeProcessCmd func(*exec.Cmd)
	// QuarantineCorruptedProfile moves profileDir aside and recreates an
	// empty dir at the same path. Used to recover from silent CDP attach
	// failures (observed with CloakBrowser when the profile dir holds
	// state it cannot ingest). Returns the quarantine path on success.
	QuarantineCorruptedProfile func(profileDir string) (string, error)
}

// InitChrome initializes a Chrome browser for a Bridge instance.
//
// When cfg.CDPAttachURL is set, this skips launching a Chrome process and
// connects to the externally-managed Chrome at that browser-level CDP
// WebSocket URL. The returned cancel funcs only release the chromedp
// allocator + browser context; the external Chrome process is left alive.
func InitChrome(cfg *config.RuntimeConfig, bundle *stealth.Bundle, hooks Hooks) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	if cfg != nil && strings.TrimSpace(cfg.CDPAttachURL) != "" {
		return initChromeFromExistingCDP(cfg, bundle)
	}

	slog.Info("starting chrome initialization", "headless", cfg.Headless, "profile", cfg.ProfileDir, "binary", cfg.ChromeBinary)
	if config.IsCloakBrowserProvider(cfg.BrowserProvider) && strings.TrimSpace(cfg.ChromeBinary) == "" {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, missingBrowserBinaryError(cfg)
	}

	bundle = ensureStealthBundle(cfg, bundle)
	geoAlignment, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to resolve proxy geo alignment: %w", err)
	}
	allocCtx, allocCancel, opts, debugPort := setupAllocator(cfg, bundle, hooks, geoAlignment)
	browserCtx, browserCancel, launchMode, err := startChrome(allocCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment)
	if err != nil {
		allocCancel()
		slog.Error("chrome initialization failed", "headless", cfg.Headless, "error", err.Error())
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to start chrome: %w", err)
	}

	if proxyAuthEnabled(cfg.Proxy) {
		if err := enableProxyAuth(browserCtx, cfg.Proxy); err != nil {
			browserCancel()
			allocCancel()
			return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to enable proxy auth: %w", err)
		}
	}

	slog.Info("chrome initialized successfully", "headless", cfg.Headless, "profile", cfg.ProfileDir)
	return allocCtx, allocCancel, browserCtx, browserCancel, launchMode, nil
}

// initChromeFromExistingCDP attaches the bridge to a Chrome that is already
// running outside pinchtab (e.g. the user's everyday browser launched with
// --remote-debugging-port=NNNN). No process is spawned and no profile lock
// is taken. The allocator is a chromedp remote allocator; the returned
// cancel funcs release only the chromedp side, never the external Chrome.
func initChromeFromExistingCDP(cfg *config.RuntimeConfig, bundle *stealth.Bundle) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	wsURL := strings.TrimSpace(cfg.CDPAttachURL)
	slog.Info("attaching to existing Chrome via CDP", "cdpUrl", wsURL)

	remoteAllocCtx, remoteAllocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	browserCtx, browserCancel := chromedp.NewContext(remoteAllocCtx)

	// Touch the browser so we fail fast if the CDP URL is unreachable. We
	// intentionally do NOT inject the stealth/UA script here — the user's
	// Chrome is theirs, and rewriting its launch contract would be both
	// surprising and likely break extensions, profile features, and
	// already-open tabs.
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return nil
	})); err != nil {
		browserCancel()
		remoteAllocCancel()
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to attach to CDP at %s: %w", wsURL, err)
	}

	slog.Info("attached to existing Chrome via CDP", "cdpUrl", wsURL)
	return remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, stealth.LaunchModeAttached, nil
}

func findChromeBinary() string {
	return FindChromeBinary()
}

// FindChromeBinary exposes the launch-time Chrome discovery used by runtime
// initialization so diagnostics can report against the same search path.
func FindChromeBinary() string {
	return browserprobe.DiscoverChromeBinary(runtimeGOOS).Found
}

func appendExecAllocatorFlag(opts []chromedp.ExecAllocatorOption, flag string) []chromedp.ExecAllocatorOption {
	name := strings.TrimPrefix(flag, "--")
	if parts := strings.SplitN(name, "=", 2); len(parts) == 2 {
		return append(opts, chromedp.Flag(parts[0], parts[1]))
	}
	return append(opts, chromedp.Flag(name, true))
}

func ensureStealthBundle(cfg *config.RuntimeConfig, bundle *stealth.Bundle) *stealth.Bundle {
	if bundle != nil {
		return bundle
	}
	return stealth.NewBundle(cfg, cryptoRandSeed())
}

func appendExecAllocatorFlags(opts []chromedp.ExecAllocatorOption, flags []string) []chromedp.ExecAllocatorOption {
	for _, flag := range flags {
		opts = appendExecAllocatorFlag(opts, flag)
	}
	return opts
}

func setupAllocator(cfg *config.RuntimeConfig, bundle *stealth.Bundle, hooks Hooks, geoAlignment launchGeoAlignment) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption, int) {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	}
	opts = appendExecAllocatorFlags(opts, BaseChromeFlagArgs())
	opts = appendExecAllocatorFlags(opts, bundle.Launch.Args)
	opts = appendExecAllocatorFlags(opts, CloakBrowserFlagArgs(cfg))
	opts = appendExecAllocatorFlags(opts, config.BrowserProxyFlags(cfg.Proxy))

	opts = appendExecAllocatorFlags(opts, geoAlignment.flags)

	chromeBinary := cfg.ChromeBinary
	if chromeBinary == "" {
		chromeBinary = findChromeBinary()
	}
	if chromeBinary != "" {
		opts = append(opts, chromedp.ExecPath(chromeBinary))
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
		// Collapse the GPU process into the browser process. Saves one
		// OS process and ~50-150MB per Chrome instance, and avoids the
		// GPU-process sandbox negotiation in our --no-sandbox containers.
		// Trade-off: a GPU code crash takes the browser with it. Disabled
		// automatically after a crash so the failure doesn't repeat.
		if !cfg.DisableInProcessGPU {
			opts = append(opts, chromedp.Flag("in-process-gpu", true))
		}
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

	opts = appendExecAllocatorFlags(opts, config.AllowedChromeExtraFlags(cfg.ChromeExtraFlags))
	for _, flag := range appendChromeCompatibilityFlags(nil) {
		opts = appendExecAllocatorFlag(opts, flag)
	}

	debugPort := 0
	if cfg.ChromeDebugPort > 0 {
		debugPort = cfg.ChromeDebugPort
		opts = append(opts, chromedp.Flag("remote-debugging-port", strconv.Itoa(debugPort)))
	} else if port, err := findFreePort(cfg.InstancePortStart, cfg.InstancePortEnd); err == nil {
		debugPort = port
		opts = append(opts, chromedp.Flag("remote-debugging-port", strconv.Itoa(port)))
	}
	opts = append(opts, chromedp.CombinedOutput(newPrefixedLogWriter(os.Stdout, "chrome")))
	opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		if len(geoAlignment.env) > 0 {
			if cmd.Env == nil {
				cmd.Env = os.Environ()
			}
			cmd.Env = mergeGeoEnv(cmd.Env, geoAlignment.env)
		}
		if hooks.ConfigureChromeProcessCmd != nil {
			hooks.ConfigureChromeProcessCmd(cmd)
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	return ctx, cancel, opts, debugPort
}

func startChrome(parentCtx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, opts []chromedp.ExecAllocatorOption, debugPort int, hooks Hooks, geoAlignment launchGeoAlignment) (context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	return startChromeWithRecovery(parentCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment, false, false)
}

func startChromeWithRecovery(parentCtx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, opts []chromedp.ExecAllocatorOption, debugPort int, hooks Hooks, geoAlignment launchGeoAlignment, retriedProfileLock, retriedProfileCorruption bool) (context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(parentCtx, opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	bundle = ensureStealthBundle(cfg, bundle)
	if hooks.SetHumanRandSeed != nil {
		hooks.SetHumanRandSeed(bundle.Seed)
	}

	const chromeStartupTimeout = 20 * time.Second
	type runResult struct{ err error }
	runCh := make(chan runResult, 1)
	startTs := time.Now()
	go func() {
		runCh <- runResult{chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return nil
		}))}
	}()

	var err error
	select {
	case res := <-runCh:
		err = res.err
	case <-time.After(chromeStartupTimeout):
		err = fmt.Errorf("chrome startup timeout after %v: %w", chromeStartupTimeout, context.DeadlineExceeded)
	}

	if err != nil {
		elapsed := time.Since(startTs)
		silentDrop := isSilentCDPAttachFailure(cfg, err, parentCtx, browserCtx, elapsed)
		browserCancel()
		allocCancel()
		errMsg := err.Error()

		if !retriedProfileLock && hooks.IsChromeProfileLockError != nil && hooks.IsChromeProfileLockError(errMsg) {
			if hooks.ClearStaleChromeProfile != nil {
				if recovered, _ := hooks.ClearStaleChromeProfile(cfg.ProfileDir, errMsg); recovered {
					time.Sleep(250 * time.Millisecond)
					return startChromeWithRecovery(parentCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment, true, retriedProfileCorruption)
				}
			}
		}

		if silentDrop && !retriedProfileCorruption && hooks.QuarantineCorruptedProfile != nil && strings.TrimSpace(cfg.ProfileDir) != "" {
			if quarantinePath, qerr := hooks.QuarantineCorruptedProfile(cfg.ProfileDir); qerr == nil {
				slog.Warn("cloakbrowser silently dropped CDP attach; quarantined profile and retrying with fresh profile",
					"originalProfile", cfg.ProfileDir,
					"quarantinedTo", quarantinePath,
					"elapsedMs", elapsed.Milliseconds())
				time.Sleep(500 * time.Millisecond)
				return startChromeWithRecovery(parentCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment, retriedProfileLock, true)
			} else {
				slog.Warn("cloakbrowser silently dropped CDP attach; profile quarantine failed",
					"originalProfile", cfg.ProfileDir,
					"err", qerr.Error())
			}
		}

		if shouldRetryChromeStartupWithDirectLaunch(parentCtx, err) && debugPort > 0 {
			slog.Warn("chrome startup failed via allocator, trying direct-launch fallback", "port", debugPort, "error", errMsg)
			time.Sleep(500 * time.Millisecond)
			return startChromeWithRemoteAllocator(parentCtx, cfg, bundle, debugPort, bundle.Script, geoAlignment)
		}

		if silentDrop {
			return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to connect to chrome: %w (cloakbrowser dropped CDP attach silently after %dms; profile %q may be corrupted — try removing it or use a fresh profile)", err, elapsed.Milliseconds(), cfg.ProfileDir)
		}
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to connect to chrome: %w", err)
	}

	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return applyStartupStealth(ctx, cfg, bundle, bundle.Script)
	})); err != nil {
		browserCancel()
		allocCancel()
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to inject stealth script: %w", err)
	}

	return browserCtx, func() {
		browserCancel()
		allocCancel()
	}, stealth.LaunchModeAllocator, nil
}

func isStartupTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "context deadline exceeded")
}

// isSilentCDPAttachFailure detects the CloakBrowser-specific pattern where
// Chrome accepts the launch, prints "DevTools listening", responds to the
// initial Target.setDiscoverTargets, but then drops the websocket between
// Target.attachToTarget and the reply — chromedp internally cancels the
// browser context and chromedp.Run returns "context canceled" within a
// few hundred milliseconds. Manifests when the profile dir holds state
// CloakBrowser refuses to ingest.
func isSilentCDPAttachFailure(cfg *config.RuntimeConfig, err error, parentCtx, browserCtx context.Context, elapsed time.Duration) bool {
	if cfg == nil || !config.IsCloakBrowserProvider(cfg.BrowserProvider) {
		return false
	}
	if err == nil {
		return false
	}
	if parentCtx != nil && parentCtx.Err() != nil {
		return false
	}
	if browserCtx == nil || browserCtx.Err() == nil {
		return false
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		return false
	}
	// The silent drop is fast — chromedp cancels within ~1s. Any later
	// failure is more likely a real timeout from a different cause.
	return elapsed < 5*time.Second
}

func shouldRetryChromeStartupWithDirectLaunch(parentCtx context.Context, err error) bool {
	if isStartupTimeout(err) {
		return true
	}
	if parentCtx != nil && parentCtx.Err() != nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return strings.Contains(err.Error(), "context canceled")
}

func startChromeWithRemoteAllocator(parentCtx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, debugPort int, injectedStealthScript string, geoAlignment launchGeoAlignment) (context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	chromeBinary := cfg.ChromeBinary
	if chromeBinary == "" {
		chromeBinary = findChromeBinary()
	}
	if chromeBinary == "" {
		return nil, nil, stealth.LaunchModeUninitialized, missingBrowserBinaryError(cfg)
	}

	args := buildChromeArgsWithBundle(cfg, bundle, debugPort, geoAlignment)
	// #nosec G204 -- chromeBinary from user config or shared browser discovery
	cmd := exec.Command(chromeBinary, args...)
	cmd.Stdout = newPrefixedLogWriter(os.Stdout, "chrome stdout")
	cmd.Stderr = newPrefixedLogWriter(os.Stderr, "chrome stderr")
	if len(geoAlignment.env) > 0 {
		cmd.Env = mergeGeoEnv(os.Environ(), geoAlignment.env)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to start chrome directly: %w", err)
	}

	// Reap the chrome process when it exits to prevent zombies.
	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	killAndReap := func() {
		_ = cmd.Process.Kill()
		<-waitDone
	}

	wsURL, err := waitForChromeDevTools(debugPort, 30*time.Second)
	if err != nil {
		killAndReap()
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("chrome devtools not ready on port %d: %w", debugPort, err)
	}

	remoteAllocCtx, remoteAllocCancel := chromedp.NewRemoteAllocator(parentCtx, wsURL)
	browserCtx, browserCancel := chromedp.NewContext(remoteAllocCtx)

	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return applyStartupStealth(ctx, cfg, bundle, injectedStealthScript)
	})); err != nil {
		browserCancel()
		remoteAllocCancel()
		killAndReap()
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to connect/inject via remote: %w", err)
	}

	return browserCtx, func() {
		browserCancel()
		remoteAllocCancel()
		killAndReap()
	}, stealth.LaunchModeDirectFallback, nil
}

func findFreePort(start, end int) (int, error) {
	for port := start; port <= end; port++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = l.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port available in range %d-%d", start, end)
}

func waitForChromeDevTools(port int, timeout time.Duration) (string, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint)
		if err == nil {
			var info struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&info)
			_ = resp.Body.Close()
			if decodeErr == nil && info.WebSocketDebuggerURL != "" {
				return info.WebSocketDebuggerURL, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", fmt.Errorf("chrome devtools not ready on port %d after %v", port, timeout)
}

func BaseChromeFlagArgs() []string {
	return []string{
		"--disable-background-networking",
		"--enable-features=NetworkService,NetworkServiceInProcess",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-breakpad",
		"--disable-session-crashed-bubble",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--disable-features=Translate,BlinkGenPropertyTrees",
		"--hide-crash-restore-bubble",
		"--disable-hang-monitor",
		"--disable-ipc-flooding-protection",
		"--disable-metrics-reporting",
		"--disable-prompt-on-repost",
		"--disable-renderer-backgrounding",
		"--disable-sync",
		"--force-color-profile=srgb",
		"--metrics-recording-only",
		"--noerrdialogs",
		"--safebrowsing-disable-auto-update",
		"--password-store=basic",
		"--use-mock-keychain",
	}
}

func appendChromeCompatibilityFlags(args []string) []string {
	if chromeNeedsNoSandbox() {
		return append(args, "--no-sandbox")
	}
	return args
}

func chromeNeedsNoSandbox() bool {
	if runtimeGOOS != "linux" {
		return false
	}
	if osGeteuid() == 0 {
		return true
	}
	if _, err := os.Stat(containerMarkerPath); err == nil {
		return true
	}
	return false
}

func BuildChromeArgs(cfg *config.RuntimeConfig, port int) []string {
	geoAlignment, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		return buildChromeArgsWithBundle(cfg, nil, port, launchGeoAlignment{})
	}
	return buildChromeArgsWithBundle(cfg, nil, port, geoAlignment)
}

func existingExtensionPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	validPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			validPaths = append(validPaths, path)
		}
	}
	return validPaths
}

func buildChromeArgsWithBundle(cfg *config.RuntimeConfig, bundle *stealth.Bundle, port int, geoAlignment launchGeoAlignment) []string {
	bundle = ensureStealthBundle(cfg, bundle)
	args := append([]string{fmt.Sprintf("--remote-debugging-port=%d", port)}, BaseChromeFlagArgs()...)
	args = append(args, bundle.Launch.Args...)
	args = append(args, CloakBrowserFlagArgs(cfg)...)
	args = append(args, config.BrowserProxyFlags(cfg.Proxy)...)
	args = append(args, geoAlignment.flags...)

	if validPaths := existingExtensionPaths(cfg.ExtensionPaths); len(validPaths) > 0 {
		joined := strings.Join(validPaths, ",")
		args = append(args, "--load-extension="+joined, "--disable-extensions-except="+joined)
	} else {
		args = append(args, "--disable-extensions")
	}

	if cfg.Headless {
		args = append(args,
			"--headless=new",
			"--disable-gpu",
			"--disable-vulkan",
			"--use-angle=swiftshader",
			"--enable-unsafe-swiftshader",
		)
	}

	if cfg.ProfileDir != "" {
		args = append(args, "--user-data-dir="+cfg.ProfileDir)
	}

	w, h := randomWindowSize()
	args = append(args, fmt.Sprintf("--window-size=%d,%d", w, h))

	if cfg.Timezone != "" {
		args = append(args, "--tz="+cfg.Timezone)
	}

	args = append(args, config.AllowedChromeExtraFlags(cfg.ChromeExtraFlags)...)

	return appendChromeCompatibilityFlags(args)
}

func CloakBrowserFlagArgs(cfg *config.RuntimeConfig) []string {
	if cfg == nil || !config.IsCloakBrowserProvider(cfg.BrowserProvider) {
		return nil
	}
	cloak := cfg.Cloak
	args := []string{}
	add := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			args = append(args, name+"="+value)
		}
	}
	add("--fingerprint", cloak.FingerprintSeed)
	add("--fingerprint-platform", cloak.Platform)
	add("--fingerprint-locale", cloak.Locale)
	add("--fingerprint-timezone", cloak.Timezone)
	add("--fingerprint-webrtc-ip", cloak.WebRTCIP)
	add("--fingerprint-fonts-dir", cloak.FontsDir)
	if cloak.StorageQuotaMB > 0 {
		args = append(args, "--fingerprint-storage-quota="+strconv.Itoa(cloak.StorageQuotaMB))
	}
	return args
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
	if cfg != nil && config.IsCloakBrowserProvider(cfg.BrowserProvider) {
		return fmt.Errorf("cloakbrowser binary not found: set browser.binary to the CloakBrowser Chromium binary when browser.provider is cloak")
	}
	return fmt.Errorf("chrome/chromium not found: please install chrome or chromium, or set 'binary' in config.json")
}

func injectedScript(ctx context.Context, script string) error {
	return chromedp.FromContext(ctx).Target.Execute(ctx,
		"Page.addScriptToEvaluateOnNewDocument",
		map[string]interface{}{
			"source": script,
		}, nil)
}

func randomWindowSize() (int, int) {
	sizes := [][2]int{
		{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
		{1280, 720}, {1600, 900}, {2560, 1440}, {1280, 800},
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(sizes))))
	idx := 0
	if err == nil {
		idx = int(n.Int64())
	}
	s := sizes[idx]
	return s[0], s[1]
}

type prefixedLogWriter struct {
	dst    io.Writer
	prefix string
	buf    []byte
}

func newPrefixedLogWriter(dst io.Writer, prefix string) *prefixedLogWriter {
	return &prefixedLogWriter{dst: dst, prefix: prefix, buf: make([]byte, 0, 1024)}
}

func (w *prefixedLogWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]
		if len(line) > 0 {
			if _, err := fmt.Fprintf(w.dst, "%s: %s\n", w.prefix, string(line)); err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
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
	if !config.CloakBrowserProviderActive(cfg) {
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
	flags, env := applyGeoAlignment(cfg.BrowserProvider, info, cfg.Cloak)
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

func cryptoRandSeed() int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000000))
	if err != nil {
		return 42
	}
	return n.Int64()
}
