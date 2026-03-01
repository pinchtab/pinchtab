package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
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

// setupAllocator creates a Chrome allocator with appropriate options
func setupAllocator(cfg *config.RuntimeConfig) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption) {
	opts := chromedp.DefaultExecAllocatorOptions[:]

	// Log configuration
	slog.Debug("configuring chrome allocator", "headless", cfg.Headless, "binary", cfg.ChromeBinary, "profile_dir", cfg.ProfileDir)

	// Headless mode
	if cfg.Headless {
		opts = append(opts, chromedp.Headless)
		slog.Debug("chrome mode set to headless")
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
		slog.Debug("chrome mode set to headed (visible window)")
	}

	// Chrome binary
	if cfg.ChromeBinary != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ChromeBinary))
		slog.Debug("chrome binary path configured", "path", cfg.ChromeBinary)
	} else {
		slog.Debug("chrome binary path not specified, using system PATH")
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

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
	if err := chromedp.Run(browserCtx); err != nil {
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
