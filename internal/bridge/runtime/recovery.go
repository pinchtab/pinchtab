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

func startBrowser(parentCtx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, opts []chromedp.ExecAllocatorOption, debugPort int, hooks Hooks, geoAlignment launchGeoAlignment) (context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	return startBrowserWithRecovery(parentCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment, false, false)
}

func startBrowserWithRecovery(parentCtx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, opts []chromedp.ExecAllocatorOption, debugPort int, hooks Hooks, geoAlignment launchGeoAlignment, retriedProfileLock, retriedProfileCorruption bool) (context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(parentCtx, opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	bundle = ensureStealthBundle(cfg, bundle)
	if hooks.SetHumanRandSeed != nil {
		hooks.SetHumanRandSeed(bundle.Seed)
	}

	const browserStartupTimeout = 20 * time.Second
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
	case <-time.After(browserStartupTimeout):
		err = fmt.Errorf("browser startup timeout after %v: %w", browserStartupTimeout, context.DeadlineExceeded)
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

		if !retriedProfileLock && hooks.IsProfileLockError != nil && hooks.IsProfileLockError(errMsg) {
			if hooks.ClearStaleProfileLocks != nil {
				if recovered, _ := hooks.ClearStaleProfileLocks(cfg.ProfileDir, errMsg); recovered {
					time.Sleep(250 * time.Millisecond)
					return startBrowserWithRecovery(parentCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment, true, retriedProfileCorruption)
				}
			}
		}

		if silentDrop && !retriedProfileCorruption && hooks.QuarantineCorruptedProfile != nil && strings.TrimSpace(cfg.ProfileDir) != "" {
			if quarantinePath, qerr := hooks.QuarantineCorruptedProfile(cfg.ProfileDir); qerr == nil {
				slog.Warn("browser silently dropped CDP attach; quarantined profile and retrying with fresh profile",
					"provider", browserID,
					"originalProfile", cfg.ProfileDir,
					"quarantinedTo", quarantinePath,
					"elapsedMs", elapsed.Milliseconds())
				time.Sleep(500 * time.Millisecond)
				return startBrowserWithRecovery(parentCtx, cfg, bundle, opts, debugPort, hooks, geoAlignment, retriedProfileLock, true)
			} else {
				slog.Warn("browser silently dropped CDP attach; profile quarantine failed",
					"provider", browserID,
					"originalProfile", cfg.ProfileDir,
					"err", qerr.Error())
			}
		}

		if shouldRetryBrowserStartupWithDirectLaunch(parentCtx, err) && debugPort > 0 {
			slog.Warn("browser startup failed via allocator, trying direct-launch fallback", "port", debugPort, "error", errMsg)
			time.Sleep(500 * time.Millisecond)
			return startBrowserWithRemoteAllocator(parentCtx, cfg, bundle, debugPort, bundle.Script, geoAlignment)
		}

		if silentDrop {
			return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to connect to browser: %w (%s dropped CDP attach silently after %dms; profile %q may be corrupted — try removing it or use a fresh profile)", err, browserID, elapsed.Milliseconds(), cfg.ProfileDir)
		}
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to connect to browser: %w", err)
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

func shouldRetryBrowserStartupWithDirectLaunch(parentCtx context.Context, err error) bool {
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

func startBrowserWithRemoteAllocator(parentCtx context.Context, cfg *config.RuntimeConfig, bundle *stealth.Bundle, debugPort int, injectedStealthScript string, geoAlignment launchGeoAlignment) (context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	plan, err := resolveProviderLaunchPlan(cfg, providerLaunchConfig(cfg, strings.TrimSpace(cfg.BrowserBinary), debugPort))
	if err != nil {
		return nil, nil, stealth.LaunchModeUninitialized, err
	}
	if plan.binary == "" {
		return nil, nil, stealth.LaunchModeUninitialized, missingBrowserBinaryError(cfg)
	}

	args, providerEnv, err := buildBrowserArgsWithBundle(cfg, bundle, debugPort, geoAlignment)
	if err != nil {
		return nil, nil, stealth.LaunchModeUninitialized, err
	}
	// #nosec G204 -- browser binary from user config or shared browser discovery
	cmd := exec.Command(plan.binary, args...)
	cmd.Stdout = newPrefixedLogWriter(os.Stdout, "browser stdout")
	cmd.Stderr = newPrefixedLogWriter(os.Stderr, "browser stderr")
	if len(providerEnv) > 0 {
		cmd.Env = mergeGeoEnv(os.Environ(), providerEnv)
	}
	if len(geoAlignment.env) > 0 {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = mergeGeoEnv(cmd.Env, geoAlignment.env)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to start browser directly: %w", err)
	}

	// Reap the browser process when it exits to prevent zombies.
	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	killAndReap := func() {
		_ = cmd.Process.Kill()
		<-waitDone
	}

	wsURL, err := waitForBrowserDevTools(debugPort, 30*time.Second)
	if err != nil {
		killAndReap()
		return nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("browser devtools not ready on port %d: %w", debugPort, err)
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

func waitForBrowserDevTools(port int, timeout time.Duration) (string, error) {
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
	return "", fmt.Errorf("browser devtools not ready on port %d after %v", port, timeout)
}
