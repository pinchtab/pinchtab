package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func setupAllocator(cfg *config.RuntimeConfig) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption) {
	if cfg.CdpURL != "" {
		slog.Info("connecting to Chrome", "url", cfg.CdpURL)
		ctx, cancel := chromedp.NewRemoteAllocator(context.Background(), cfg.CdpURL)
		return ctx, cancel, nil
	}

	if err := os.MkdirAll(cfg.ProfileDir, 0755); err != nil {
		slog.Error("cannot create profile dir", "err", err)
		os.Exit(1)
	}

	for _, lockName := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		lockPath := fmt.Sprintf("%s/%s", cfg.ProfileDir, lockName)
		if err := os.Remove(lockPath); err == nil {
			slog.Warn("removed stale lock", "file", lockName)
		}
	}

	if bridge.WasUncleanExit(cfg.ProfileDir) {
		slog.Warn("previous session exited uncleanly, clearing Chrome session restore data")
		bridge.ClearChromeSessions(cfg.ProfileDir)
	}

	slog.Info("launching Chrome", "profile", cfg.ProfileDir, "headless", cfg.Headless)

	opts := buildChromeOpts(cfg)

	bridge.MarkCleanExit(cfg.ProfileDir)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return ctx, cancel, opts
}

func buildChromeOpts(cfg *config.RuntimeConfig) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.UserDataDir(cfg.ProfileDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,

		chromedp.Flag("exclude-switches", "enable-automation"),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-web-security", false),

		chromedp.Flag("disable-background-networking", false),
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("no-first-run", true),

		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.Flag("hide-crash-restore-bubble", true),
		chromedp.Flag("disable-device-discovery-notifications", true),

		chromedp.Flag("js-flags", "--random-seed=1157259157"),

		chromedp.WindowSize(randomWindowSize()),
	}

	if cfg.ChromeBinary != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ChromeBinary))
	}
	if cfg.ChromeExtraFlags != "" {
		for _, f := range strings.Fields(cfg.ChromeExtraFlags) {
			if k, v, ok := strings.Cut(f, "="); ok {
				opts = append(opts, chromedp.Flag(strings.TrimLeft(k, "-"), v))
			} else {
				opts = append(opts, chromedp.Flag(strings.TrimLeft(f, "-"), true))
			}
		}
	}

	if cfg.Headless {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	return opts
}

func startChrome(allocCtx context.Context, seededScript string) (context.Context, context.CancelFunc, error) {
	bCtx, bCancel := chromedp.NewContext(allocCtx)

	startCtx, startDone := context.WithTimeout(context.Background(), chromeStartTimeout)
	defer startDone()

	errCh := make(chan error, 1)
	go func() {
		errCh <- chromedp.Run(bCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := page.AddScriptToEvaluateOnNewDocument(seededScript).Do(ctx)
				return err
			}),
		)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			bCancel()
			return nil, nil, err
		}
		return bCtx, bCancel, nil
	case <-startCtx.Done():
		bCancel()
		return nil, nil, fmt.Errorf("timed out after %s", chromeStartTimeout)
	}
}

func applyTimezone(browserCtx context.Context, cfg *config.RuntimeConfig) {
	if cfg.Timezone == "" {
		return
	}
	if err := chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetTimezoneOverride(cfg.Timezone).Do(ctx)
		}),
	); err != nil {
		slog.Warn("timezone override failed", "tz", cfg.Timezone, "err", err)
	} else {
		slog.Info("timezone override", "tz", cfg.Timezone)
	}
}
