package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

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
		browserID := config.NormalizeBrowser(cfg.DefaultBrowser)
		silentDrop := false
		if b, ok := browsers.Get(browserID); ok {
			kind := b.ClassifyLaunchError(browsers.LaunchFailure{
				Err:             err,
				Elapsed:         elapsed,
				ParentCanceled:  parentCtx != nil && parentCtx.Err() != nil,
				BrowserCanceled: browserCtx != nil && browserCtx.Err() != nil,
			})
			silentDrop = kind == browsers.LaunchErrorSilentCDPDrop
		}
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
