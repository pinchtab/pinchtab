package runtime

import (
	"context"
	"crypto/rand"
	"log/slog"
	"math/big"
	"os"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func appendExecAllocatorFlag(opts []chromedp.ExecAllocatorOption, flag string) []chromedp.ExecAllocatorOption {
	name := strings.TrimPrefix(flag, "--")
	if parts := strings.SplitN(name, "=", 2); len(parts) == 2 {
		return append(opts, chromedp.Flag(parts[0], parts[1]))
	}
	return append(opts, chromedp.Flag(name, true))
}

func appendExecAllocatorFlags(opts []chromedp.ExecAllocatorOption, flags []string) []chromedp.ExecAllocatorOption {
	for _, flag := range flags {
		opts = appendExecAllocatorFlag(opts, flag)
	}
	return opts
}

func browserLaunchArgs(headless bool) []string {
	return runtimekit.BaseFlagArgs("chrome", headless)
}

func BaseBrowserFlagArgs() []string {
	return browserLaunchArgs(false)
}

func appendBrowserCompatibilityFlags(args []string) []string {
	if launchNeedsNoSandbox() {
		return append(args, "--no-sandbox")
	}
	return args
}

func launchNeedsNoSandbox() bool {
	_, err := os.Stat(containerMarkerPath)
	return runtimekit.ChromeNeedsNoSandbox(runtimeGOOS, osGeteuid(), err == nil)
}

func BuildBrowserArgs(cfg *config.RuntimeConfig, port int) []string {
	geoAlignment, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		args, _, buildErr := buildBrowserArgsWithBundle(cfg, nil, port, launchGeoAlignment{})
		if buildErr != nil {
			slog.Error("build browser args failed", "err", buildErr)
			return nil
		}
		return args
	}
	args, _, err := buildBrowserArgsWithBundle(cfg, nil, port, geoAlignment)
	if err != nil {
		slog.Error("build browser args failed", "err", err)
		return nil
	}
	return args
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

func buildBrowserArgsWithBundle(cfg *config.RuntimeConfig, bundle *stealth.Bundle, port int, geoAlignment launchGeoAlignment) ([]string, []string, error) {
	bundle = ensureStealthBundle(cfg, bundle)
	binary := strings.TrimSpace(cfg.BrowserBinary)
	if binary == "" {
		binary = runtimekit.FindBrowserBinary(config.NormalizeBrowser(cfg.DefaultBrowser))
	}
	launchCfg := runtimekit.LaunchConfigFromRuntime(cfg, binary, port, launchNeedsNoSandbox())
	if cfg.DisableInProcessGPU {
		// Crash-recovery kill switch: a user-supplied --in-process-gpu must
		// not survive a retry after a GPU crash loop.
		launchCfg.ExtraFlags = stripInProcessGPUFlag(launchCfg.ExtraFlags)
	}
	plan, err := resolveProviderLaunchPlan(cfg, launchCfg)
	if err != nil {
		return nil, nil, err
	}
	args := append([]string(nil), plan.args...)

	args = append(args, bundle.Launch.Args...)
	proxyFlags, err := config.BrowserProxyFlags(cfg.Proxy)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, proxyFlags...)
	args = append(args, geoAlignment.flags...)

	env := append([]string(nil), plan.env...)
	return args, env, nil
}

func CloakBrowserFlagArgs(cfg *config.RuntimeConfig) []string {
	return runtimekit.CloakBrowserFlagArgs(cfg)
}

func stripInProcessGPUFlag(flags []string) []string {
	out := flags[:0]
	for _, f := range flags {
		name := strings.SplitN(f, "=", 2)[0]
		if strings.EqualFold(name, "--in-process-gpu") {
			continue
		}
		out = append(out, f)
	}
	return out
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

func cryptoRandSeed() int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000000))
	if err != nil {
		return 42
	}
	return n.Int64()
}
