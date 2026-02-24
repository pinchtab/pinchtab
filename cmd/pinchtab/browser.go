package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
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

	killOrphanedChrome(cfg.ProfileDir)

	slog.Info("launching Chrome", "profile", cfg.ProfileDir, "headless", cfg.Headless)

	opts := buildChromeOpts(cfg)

	bridge.MarkCleanExit(cfg.ProfileDir)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return ctx, cancel, opts
}

// killOrphanedChrome finds and kills Chrome processes that were spawned by a
// previous Pinchtab instance using the same profile directory. We identify our
// processes by matching --user-data-dir=<profileDir> in the command line args,
// which is unique to Pinchtab-managed Chrome and won't match the user's Chrome.
func killOrphanedChrome(profileDir string) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	// Use ps to find Chrome processes with our profile dir in the args
	marker := "--user-data-dir=" + profileDir
	out, err := exec.Command("ps", "axo", "pid,args").Output()
	if err != nil {
		return
	}

	var killed int
	for _, line := range bytes.Split(out, []byte("\n")) {
		lineStr := strings.TrimSpace(string(line))
		if !strings.Contains(lineStr, marker) {
			continue
		}
		parts := strings.SplitN(lineStr, " ", 2)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil || pid == os.Getpid() {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.SIGTERM); err == nil {
				killed++
			}
		}
	}
	if killed > 0 {
		slog.Warn("killed orphaned Chrome processes", "count", killed, "profileDir", profileDir)
	}
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

func startChrome(allocCtx context.Context, cfg *config.RuntimeConfig, seededScript string) (context.Context, context.CancelFunc, error) {
	startCtx, startDone := context.WithTimeout(context.Background(), chromeStartTimeout)
	defer startDone()

	// For CDP_URL (remote allocator), ensure we have a target available.
	// Remote Chrome instances may not have any windows open yet.
	if cfg.CdpURL != "" {
		// Get or create a target for the remote browser
		getCtx, getCancel := context.WithTimeout(context.Background(), 5*time.Second)
		targets, err := func() ([]*target.Info, error) {
			var tgts []*target.Info
			getErr := chromedp.Run(getCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					tgts, err = target.GetTargets().Do(ctx)
					return err
				}),
			)
			return tgts, getErr
		}()
		getCancel()

		targetID := target.ID("")
		if err == nil && len(targets) > 0 {
			// Use first existing target
			targetID = targets[0].TargetID
		} else {
			// Try to create a new target
			createCtx, createCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = chromedp.Run(createCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					tID, err := target.CreateTarget("about:blank").Do(ctx)
					if err == nil {
						targetID = tID
					}
					return nil // Don't fail, will try with browser context
				}),
			)
			createCancel()
		}

		bCtx, bCancel := chromedp.NewContext(allocCtx,
			chromedp.WithTargetID(targetID),
		)
		return bCtx, bCancel, nil
	}

	bCtx, bCancel := chromedp.NewContext(allocCtx)

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
