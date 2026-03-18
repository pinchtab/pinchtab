package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"runtime"

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
	allocCtx, allocCancel, opts := setupAllocator(cfg)
	slog.Debug("chrome allocator configured", "headless", cfg.Headless)

	// Start Chrome browser
	browserCtx, browserCancel, err := startChrome(allocCtx, cfg, opts)
	if err != nil {
		allocCancel()
		slog.Error("chrome initialization failed", "headless", cfg.Headless, "error", err.Error())
		return nil, nil, nil, nil, fmt.Errorf("failed to start chrome: %w", err)
	}

	slog.Info("chrome initialized successfully", "headless", cfg.Headless, "profile", cfg.ProfileDir)
	return allocCtx, allocCancel, browserCtx, browserCancel, nil
}

// findChromeBinary searches for Chrome in common installation locations
func findChromeBinary() string {
	// Common Chrome/Chromium binary locations by OS
	candidates := []string{
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

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			slog.Debug("found chrome binary", "path", path)
			return path
		}
	}

	return ""
}

// setupAllocator creates a Chrome allocator with appropriate options
func setupAllocator(cfg *config.RuntimeConfig) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption) {
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

	// Headless mode — use Chrome's "new" headless mode (--headless=new) which:
	// 1. Reports regular Chrome UA (not "HeadlessChrome") everywhere including Service Workers
	// 2. Properly emulates screen dimensions (not default 800×600)
	// 3. Simulates window decorations (taskbar/dock) so screenFrame isn't [0,0,0,0]
	// Old --headless mode leaked via Service Worker UA, detected by CreepJS at 67% headless.
	// See: d_20260318_045
	if cfg.Headless {
		opts = append(opts, chromedp.Flag("headless", "new"))
		slog.Debug("chrome mode set to headless (new mode)")
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

	// Stealth: override --enable-automation from chromedp defaults.
	// enable-automation sets navigator.webdriver=true and shows the automation bar.
	// Setting it to false here overrides the default (chromedp uses a map, last write wins).
	opts = append(opts,
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-dev-shm-usage", ""),
		chromedp.Flag("no-first-run", ""),
		chromedp.Flag("no-default-browser-check", ""),
	)

	// Stealth: set --user-agent flag to override UA globally (including Service Workers).
	// CDP emulation.SetUserAgentOverride only affects the page target, not Service Worker
	// targets. The --user-agent flag overrides the binary's built-in UA at the network level.
	// Without this, Service Workers report "HeadlessChrome/VERSION" even in --headless=new.
	if cfg.ChromeVersion != "" {
		var ua string
		switch runtime.GOOS {
		case "darwin":
			ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + cfg.ChromeVersion + " Safari/537.36"
		case "windows":
			ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + cfg.ChromeVersion + " Safari/537.36"
		default:
			ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + cfg.ChromeVersion + " Safari/537.36"
		}
		opts = append(opts, chromedp.UserAgent(ua))
		slog.Debug("user-agent override applied globally", "version", cfg.ChromeVersion)
	}

	// Extra flags
	if cfg.ChromeExtraFlags != "" {
		opts = append(opts, chromedp.Flag("", cfg.ChromeExtraFlags))
	}

	// Timezone
	if cfg.Timezone != "" {
		opts = append(opts, chromedp.Flag("TZ", cfg.Timezone))
	}

	// Allocator/browser context must be long-lived for the entire bridge instance.
	// A short timeout here causes all later tab creation to fail once the deadline expires.
	ctx, cancel := context.WithCancel(context.Background())

	return ctx, cancel, opts
}

// startChrome starts the Chrome browser with the given options
func startChrome(allocCtx context.Context, cfg *config.RuntimeConfig, opts []chromedp.ExecAllocatorOption) (context.Context, context.CancelFunc, error) {
	slog.Debug("creating chrome allocator")

	// Create allocator context
	allocCtx, allocCancel := chromedp.NewExecAllocator(allocCtx, opts...)
	slog.Debug("chrome allocator created")

	// Create browser context
	slog.Debug("creating chrome context")
	browserCtx, cancel := chromedp.NewContext(allocCtx)

	// Initialize stealth script
	stealthSeed := rand.Intn(1000000000)
	human.SetHumanRandSeed(int64(stealthSeed))
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", stealthSeed, cfg.StealthLevel) + assets.StealthScript

	// Start browser (connect to Chrome)
	slog.Debug("connecting to chrome browser")

	// The browserCtx should now connect to the Chrome process started by the allocator
	// Run a simple action to verify the connection
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		slog.Debug("chrome connection established, running initial action")
		return nil
	})); err != nil {
		cancel()
		allocCancel()
		slog.Error("failed to connect to chrome browser", "error", err.Error())
		return nil, nil, fmt.Errorf("failed to connect to chrome: %w", err)
	}
	slog.Debug("chrome browser connected successfully")

	// Inject stealth script
	slog.Debug("injecting stealth script")
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return injectedScript(ctx, seededScript)
	})); err != nil {
		cancel()
		allocCancel()
		slog.Error("failed to inject stealth script", "error", err.Error())
		return nil, nil, fmt.Errorf("failed to inject stealth script: %w", err)
	}
	slog.Debug("stealth script injected successfully")

	return browserCtx, func() {
		cancel()
		allocCancel()
	}, nil
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
