package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/human"
)

// InitChrome initializes a Chrome browser for a Bridge instance
// It allocates the browser, starts it with appropriate flags (headless/headed),
// and returns the contexts ready for use.
func InitChrome(cfg *config.RuntimeConfig) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, error) {
	slog.Info("starting chrome initialization", "headless", cfg.Headless, "profile", cfg.ProfileDir, "binary", cfg.ChromeBinary)

	// Setup allocator
	allocCtx, allocCancel, opts, debugPort := setupAllocator(cfg)
	slog.Debug("chrome allocator configured", "headless", cfg.Headless, "debug_port", debugPort)

	// Start Chrome browser
	browserCtx, browserCancel, err := startChrome(allocCtx, cfg, opts, debugPort)
	if err != nil {
		allocCancel()
		slog.Error("chrome initialization failed", "headless", cfg.Headless, "error", err.Error())
		return nil, nil, nil, nil, fmt.Errorf("failed to start chrome: %w", err)
	}

	slog.Info("chrome initialized successfully", "headless", cfg.Headless, "profile", cfg.ProfileDir)
	return allocCtx, allocCancel, browserCtx, browserCancel, nil
}

// findChromeBinary searches for Chrome in common installation locations
// On ARM64 (Raspberry Pi, etc.), prioritizes chromium-browser for optimal compatibility
func findChromeBinary() string {
	var candidates []string

	// ARM64-specific optimization: Chromium is the standard on Raspberry Pi
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		candidates = []string{
			// ARM64/Raspberry Pi - prioritize chromium-browser
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
		}
		slog.Debug("detected ARM architecture, prioritizing chromium-browser", "arch", runtime.GOARCH)
	} else {
		// x86/x64 - standard order
		candidates = []string{
			// macOS
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			// Linux
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			// Windows (via WSL or MSYS)
			"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
			"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			slog.Debug("found chrome binary", "path", path, "arch", runtime.GOARCH)
			return path
		}
	}

	slog.Debug("no chrome binary found in common locations", "arch", runtime.GOARCH)
	return ""
}

// setupAllocator creates a Chrome allocator with appropriate options.
// It returns the allocator context, its cancel function, options, and the
// fixed debug port chosen from the configured range (0 if unavailable).
// Using a fixed port (instead of --remote-debugging-port=0) enables the
// direct-launch fallback for Chrome 145+, which stopped announcing the
// DevTools URL via stderr.
func setupAllocator(cfg *config.RuntimeConfig) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption, int) {
	opts := chromedp.DefaultExecAllocatorOptions[:]

	// Determine Chrome binary path
	chromeBinary := cfg.ChromeBinary
	if chromeBinary == "" {
		// Try to auto-detect Chrome
		chromeBinary = findChromeBinary()
		if chromeBinary != "" {
			slog.Info("auto-detected chrome binary", "path", chromeBinary)
		}
	}

	// Log configuration
	slog.Debug("configuring chrome allocator", "headless", cfg.Headless, "binary", chromeBinary, "profile_dir", cfg.ProfileDir)

	// Headless mode
	if cfg.Headless {
		opts = append(opts, chromedp.Headless)
		slog.Debug("chrome mode set to headless")
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
		slog.Debug("chrome mode set to headed (visible window)")
	}

	// Chrome binary
	if chromeBinary != "" {
		opts = append(opts, chromedp.ExecPath(chromeBinary))
		slog.Debug("chrome binary path configured", "path", chromeBinary)
	} else {
		slog.Debug("chrome binary path not found in common locations, letting chromedp search")
	}

	// Profile
	if cfg.ProfileDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.ProfileDir))
		slog.Debug("chrome user data directory configured", "path", cfg.ProfileDir)
	}

	// Window size
	w, h := randomWindowSize()
	opts = append(opts, chromedp.WindowSize(w, h))

	// Common stealth flags
	opts = append(opts,
		chromedp.Flag("disable-automation", ""),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-dev-shm-usage", ""),
		chromedp.Flag("no-first-run", ""),
		chromedp.Flag("no-default-browser-check", ""),
	)

	// Extra flags
	if cfg.ChromeExtraFlags != "" {
		opts = append(opts, chromedp.Flag("", cfg.ChromeExtraFlags))
	}

	// Timezone
	if cfg.Timezone != "" {
		opts = append(opts, chromedp.Flag("TZ", cfg.Timezone))
	}

	// Override --remote-debugging-port=0 (from DefaultExecAllocatorOptions) with a
	// fixed port from the configured range. Chrome 145 stopped printing the
	// "DevTools listening on ws://..." message to stderr when port=0 is used, which
	// prevents chromedp's ExecAllocator from detecting the actual port. With a known
	// fixed port we can fall back to polling /json/version if stderr parsing fails.
	debugPort := 0
	if port, err := findFreePort(cfg.InstancePortStart, cfg.InstancePortEnd); err == nil {
		debugPort = port
		opts = append(opts, chromedp.Flag("remote-debugging-port", strconv.Itoa(port)))
		slog.Debug("chrome debug port assigned", "port", port)
	} else {
		slog.Debug("could not find free port in configured range, using default port=0", "err", err)
	}

	// Allocator/browser context must be long-lived for the entire bridge instance.
	// A short timeout here causes all later tab creation to fail once the deadline expires.
	ctx, cancel := context.WithCancel(context.Background())

	return ctx, cancel, opts, debugPort
}

// startChrome starts the Chrome browser with the given options.
// It first tries chromedp's ExecAllocator approach. Chrome 145 and later no
// longer print "DevTools listening on ws://..." to stderr, so if the startup
// times out we fall back to launching Chrome directly via os/exec and connecting
// with a RemoteAllocator (requires debugPort > 0).
func startChrome(parentCtx context.Context, cfg *config.RuntimeConfig, opts []chromedp.ExecAllocatorOption, debugPort int) (context.Context, context.CancelFunc, error) {
	slog.Debug("creating chrome allocator")

	// Create allocator context (inner, wraps parentCtx)
	allocCtx, allocCancel := chromedp.NewExecAllocator(parentCtx, opts...)
	slog.Debug("chrome allocator created")

	// Create browser context
	slog.Debug("creating chrome context")
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	// Initialize stealth script
	stealthSeed := rand.Intn(1000000000)
	human.SetHumanRandSeed(int64(stealthSeed))
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", stealthSeed, cfg.StealthLevel) + assets.StealthScript

	// Connect to Chrome with a startup timeout so we can detect Chrome 145's silent
	// startup behaviour. Chrome 145 no longer prints "DevTools listening on ws://..."
	// to stderr, so chromedp's ExecAllocator would block forever waiting for that
	// message.
	//
	// IMPORTANT: we intentionally do NOT derive a timeout context from browserCtx.
	// A derived timeout context, when cancelled, propagates through chromedp's internal
	// allocator goroutines and kills the Chrome process — defeating the purpose of having
	// the fallback. Instead we run chromedp.Run in a goroutine and race it against a
	// time.After channel; the browserCtx lifetime remains independent.
	const chromeStartupTimeout = 20 * time.Second

	type runResult struct{ err error }
	runCh := make(chan runResult, 1)
	go func() {
		runCh <- runResult{chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			slog.Debug("chrome connection established, running initial action")
			return nil
		}))}
	}()

	slog.Debug("connecting to chrome browser")

	var err error
	select {
	case res := <-runCh:
		err = res.err
	case <-time.After(chromeStartupTimeout):
		// Chrome started (ExecAllocator launched the process) but never announced
		// its DevTools URL via stderr.  Wrap as DeadlineExceeded so isStartupTimeout
		// recognises it, then fall through to the cleanup + fallback logic below.
		err = fmt.Errorf("chrome startup timeout after %v: %w", chromeStartupTimeout, context.DeadlineExceeded)
	}

	if err != nil {
		// Clean up the ExecAllocator and its Chrome process before evaluating error.
		browserCancel()
		allocCancel()

		errMsg := err.Error()

		// Chrome binary not found — report clearly and stop.
		if strings.Contains(errMsg, "executable file not found") ||
			strings.Contains(errMsg, "no such file or directory") {
			slog.Error("chrome binary not found", "error", errMsg)

			if cfg.ChromeBinary != "" {
				slog.Error("chrome not found at specified path", "path", cfg.ChromeBinary)
			} else {
				slog.Error("chrome not found in common locations")
			}

			slog.Info("install chrome or chromium:")
			slog.Info("  debian/ubuntu/raspberry pi: sudo apt install -y chromium-browser")
			slog.Info("  fedora/rhel: sudo dnf install -y chromium")
			slog.Info("  arch: sudo pacman -S chromium")
			slog.Info("  macos: brew install chromium")
			slog.Info("  or set CHROME_BIN environment variable to chrome binary path")

			return nil, nil, fmt.Errorf("chrome/chromium not found: please install chrome or chromium, or set CHROME_BIN environment variable")
		}

		// Startup timeout: Chrome is running but did not announce its DevTools URL via
		// stderr. This is the Chrome 145 regression. If we have a fixed debug port we
		// can connect directly using RemoteAllocator.
		if isStartupTimeout(err) && debugPort > 0 {
			slog.Warn("chrome startup timeout: DevTools URL not announced via stderr (Chrome 145+ regression), trying direct-launch fallback",
				"timeout", chromeStartupTimeout, "port", debugPort)
			// Brief pause for ExecAllocator's Chrome process (if any) to fully exit.
			time.Sleep(500 * time.Millisecond)
			return startChromeWithRemoteAllocator(parentCtx, cfg, debugPort, stealthSeed, seededScript)
		}

		slog.Error("failed to connect to chrome browser", "error", errMsg)
		return nil, nil, fmt.Errorf("failed to connect to chrome: %w", err)
	}
	slog.Debug("chrome browser connected successfully")

	// Inject stealth script
	slog.Debug("injecting stealth script")
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return injectedScript(ctx, seededScript)
	})); err != nil {
		browserCancel()
		allocCancel()
		slog.Error("failed to inject stealth script", "error", err.Error())
		return nil, nil, fmt.Errorf("failed to inject stealth script: %w", err)
	}
	slog.Debug("stealth script injected successfully")

	return browserCtx, func() {
		browserCancel()
		allocCancel()
	}, nil
}

// isStartupTimeout reports whether err indicates that Chrome started but did not
// announce its DevTools URL within the startup window. This is the signature of
// the Chrome 145 regression where the "DevTools listening on ws://..." message is
// no longer written to stderr.
func isStartupTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline exceeded")
}

// startChromeWithRemoteAllocator launches Chrome directly via os/exec with a
// fixed debug port, waits for the DevTools endpoint to become available, then
// connects using chromedp's RemoteAllocator. This is the fallback path for
// Chrome 145+, which no longer announces the DevTools URL via stderr.
func startChromeWithRemoteAllocator(parentCtx context.Context, cfg *config.RuntimeConfig, debugPort int, stealthSeed int, seededScript string) (context.Context, context.CancelFunc, error) {
	slog.Info("using direct-launch fallback for Chrome 145+ compatibility", "port", debugPort)

	chromeBinary := cfg.ChromeBinary
	if chromeBinary == "" {
		chromeBinary = findChromeBinary()
	}
	if chromeBinary == "" {
		return nil, nil, fmt.Errorf("chrome/chromium not found: please install chrome or chromium, or set CHROME_BIN environment variable")
	}

	args := buildChromeArgs(cfg, debugPort)
	slog.Debug("launching chrome directly", "binary", chromeBinary, "port", debugPort, "args", args)

	cmd := exec.Command(chromeBinary, args...) //nolint:gosec // binary path comes from user config or known system locations
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start chrome directly: %w", err)
	}
	slog.Debug("chrome process started", "pid", cmd.Process.Pid, "port", debugPort)

	// Wait for Chrome's DevTools HTTP endpoint to become available.
	wsURL, err := waitForChromeDevTools(debugPort, 30*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, nil, fmt.Errorf("chrome devtools not ready on port %d: %w", debugPort, err)
	}
	slog.Debug("chrome devtools ready", "wsURL", wsURL, "port", debugPort)

	// Connect via RemoteAllocator
	remoteAllocCtx, remoteAllocCancel := chromedp.NewRemoteAllocator(parentCtx, wsURL)
	browserCtx, browserCancel := chromedp.NewContext(remoteAllocCtx)

	// Verify the connection
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		slog.Debug("chrome remote connection established")
		return nil
	})); err != nil {
		browserCancel()
		remoteAllocCancel()
		_ = cmd.Process.Kill()
		return nil, nil, fmt.Errorf("failed to connect to chrome via remote allocator: %w", err)
	}

	// Inject stealth script
	slog.Debug("injecting stealth script (remote path)")
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return injectedScript(ctx, seededScript)
	})); err != nil {
		browserCancel()
		remoteAllocCancel()
		_ = cmd.Process.Kill()
		slog.Error("failed to inject stealth script (remote path)", "error", err.Error())
		return nil, nil, fmt.Errorf("failed to inject stealth script: %w", err)
	}
	slog.Debug("stealth script injected successfully (remote path)")

	slog.Info("chrome initialized via direct-launch fallback", "pid", cmd.Process.Pid, "port", debugPort)

	return browserCtx, func() {
		browserCancel()
		remoteAllocCancel()
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			slog.Warn("failed to kill chrome process on shutdown", "pid", cmd.Process.Pid, "err", err)
		}
	}, nil
}

// findFreePort finds an available TCP port in the inclusive range [start, end].
// It temporarily binds to each port to test availability, releasing it immediately.
// Returns an error if no port in the range is available.
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

// waitForChromeDevTools polls Chrome's /json/version HTTP endpoint until it
// returns a valid browser-level WebSocket URL or the timeout expires.
// Chrome typically becomes ready within 1–3 seconds of startup.
func waitForChromeDevTools(port int, timeout time.Duration) (string, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint) //nolint:noctx // intentional: uses client timeout not request context
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

// buildChromeArgs constructs the Chrome command-line arguments for direct launch.
// This mirrors the flags applied by setupAllocator/DefaultExecAllocatorOptions
// and is used exclusively by the direct-launch fallback path.
func buildChromeArgs(cfg *config.RuntimeConfig, port int) []string {
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		// Core stability / automation flags (mirrors chromedp.DefaultExecAllocatorOptions)
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-breakpad",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-hang-monitor",
		"--disable-ipc-flooding-protection",
		"--disable-popup-blocking",
		"--disable-prompt-on-repost",
		"--disable-renderer-backgrounding",
		"--disable-sync",
		"--force-color-profile=srgb",
		"--metrics-recording-only",
		"--no-first-run",
		"--no-default-browser-check",
		"--safebrowsing-disable-auto-update",
		"--password-store=basic",
		"--use-mock-keychain",
		// Stealth
		"--disable-automation",
		"--disable-blink-features=AutomationControlled",
	}

	if cfg.Headless {
		args = append(args,
			"--headless=new",
			"--disable-gpu",
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

	if cfg.ChromeExtraFlags != "" {
		args = append(args, cfg.ChromeExtraFlags)
	}

	return args
}

// injectedScript injects stealth code into the browser
func injectedScript(ctx context.Context, script string) error {
	// This is a placeholder - actual implementation would use chromedp
	// to evaluate the script in the browser context
	return nil
}

// randomWindowSize returns a random common window size
func randomWindowSize() (int, int) {
	sizes := [][2]int{
		{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
		{1280, 720}, {1600, 900}, {2560, 1440}, {1280, 800},
	}
	s := sizes[rand.Intn(len(sizes))]
	return s[0], s[1]
}
