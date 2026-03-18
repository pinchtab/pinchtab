package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/config"
)

type Hooks struct {
	SetHumanRandSeed          func(int64)
	IsChromeProfileLockError  func(string) bool
	ClearStaleChromeProfile   func(profileDir, errMsg string) (bool, error)
	ConfigureChromeProcessCmd func(*exec.Cmd)
}

// InitChrome initializes a Chrome browser for a Bridge instance.
func InitChrome(cfg *config.RuntimeConfig, hooks Hooks) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, error) {
	slog.Info("starting chrome initialization", "headless", cfg.Headless, "profile", cfg.ProfileDir, "binary", cfg.ChromeBinary)

	allocCtx, allocCancel, opts, debugPort := setupAllocator(cfg, hooks)
	browserCtx, browserCancel, err := startChrome(allocCtx, cfg, opts, debugPort, hooks)
	if err != nil {
		allocCancel()
		slog.Error("chrome initialization failed", "headless", cfg.Headless, "error", err.Error())
		return nil, nil, nil, nil, fmt.Errorf("failed to start chrome: %w", err)
	}

	slog.Info("chrome initialized successfully", "headless", cfg.Headless, "profile", cfg.ProfileDir)
	return allocCtx, allocCancel, browserCtx, browserCancel, nil
}

func findChromeBinary() string {
	var candidates []string
	if goruntime.GOARCH == "arm64" || goruntime.GOARCH == "arm" {
		candidates = []string{
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
		}
	} else {
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
			"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func appendExecAllocatorFlag(opts []chromedp.ExecAllocatorOption, flag string) []chromedp.ExecAllocatorOption {
	name := strings.TrimPrefix(flag, "--")
	if parts := strings.SplitN(name, "=", 2); len(parts) == 2 {
		return append(opts, chromedp.Flag(parts[0], parts[1]))
	}
	return append(opts, chromedp.Flag(name, true))
}

func setupAllocator(cfg *config.RuntimeConfig, hooks Hooks) (context.Context, context.CancelFunc, []chromedp.ExecAllocatorOption, int) {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	}
	for _, flag := range DefaultChromeFlagArgs() {
		opts = appendExecAllocatorFlag(opts, flag)
	}

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
		opts = append(opts, chromedp.Flag("use-angle", "swiftshader"))
		opts = append(opts, chromedp.DisableGPU)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	if len(cfg.ExtensionPaths) > 0 {
		var validPaths []string
		for _, path := range cfg.ExtensionPaths {
			if _, err := os.Stat(path); err == nil {
				validPaths = append(validPaths, path)
			}
		}
		if len(validPaths) > 0 {
			joined := strings.Join(validPaths, ",")
			opts = append(opts, chromedp.Flag("disable-extensions", false))
			opts = append(opts, chromedp.Flag("load-extension", joined))
			opts = append(opts, chromedp.Flag("disable-extensions-except", joined))
			opts = append(opts, chromedp.Flag("enable-automation", false))
			slog.Info("loading extensions", "paths", joined)
		}
		opts = append(opts, chromedp.Flag("enable-automation", false))
	} else {
		opts = append(opts, chromedp.Flag("disable-extensions", true))
		opts = append(opts, chromedp.Flag("enable-automation", false))
	}

	if cfg.ProfileDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.ProfileDir))
	}

	w, h := randomWindowSize()
	opts = append(opts, chromedp.WindowSize(w, h))

	if cfg.Timezone != "" {
		opts = append(opts, chromedp.Flag("tz", cfg.Timezone))
	}

	if cfg.ChromeExtraFlags != "" {
		for _, f := range strings.Fields(cfg.ChromeExtraFlags) {
			opts = appendExecAllocatorFlag(opts, f)
		}
	}
	for _, flag := range appendChromeCompatibilityFlags(nil) {
		opts = appendExecAllocatorFlag(opts, flag)
	}

	debugPort := 0
	if port, err := findFreePort(cfg.InstancePortStart, cfg.InstancePortEnd); err == nil {
		debugPort = port
		opts = append(opts, chromedp.Flag("remote-debugging-port", strconv.Itoa(port)))
	}
	opts = append(opts, chromedp.CombinedOutput(newPrefixedLogWriter(os.Stdout, "chrome")))
	opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		if hooks.ConfigureChromeProcessCmd != nil {
			hooks.ConfigureChromeProcessCmd(cmd)
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	return ctx, cancel, opts, debugPort
}

func startChrome(parentCtx context.Context, cfg *config.RuntimeConfig, opts []chromedp.ExecAllocatorOption, debugPort int, hooks Hooks) (context.Context, context.CancelFunc, error) {
	return startChromeWithRecovery(parentCtx, cfg, opts, debugPort, hooks, false)
}

func startChromeWithRecovery(parentCtx context.Context, cfg *config.RuntimeConfig, opts []chromedp.ExecAllocatorOption, debugPort int, hooks Hooks, retriedProfileLock bool) (context.Context, context.CancelFunc, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(parentCtx, opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	stealthSeed := rand.Intn(1000000000)
	if hooks.SetHumanRandSeed != nil {
		hooks.SetHumanRandSeed(int64(stealthSeed))
	}
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", stealthSeed, cfg.StealthLevel) + assets.StealthScript + "\n" + PopupGuardInitScript

	const chromeStartupTimeout = 20 * time.Second
	type runResult struct{ err error }
	runCh := make(chan runResult, 1)
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
		browserCancel()
		allocCancel()
		errMsg := err.Error()

		if !retriedProfileLock && hooks.IsChromeProfileLockError != nil && hooks.IsChromeProfileLockError(errMsg) {
			if hooks.ClearStaleChromeProfile != nil {
				if recovered, _ := hooks.ClearStaleChromeProfile(cfg.ProfileDir, errMsg); recovered {
					time.Sleep(250 * time.Millisecond)
					return startChromeWithRecovery(parentCtx, cfg, opts, debugPort, hooks, true)
				}
			}
		}

		if isStartupTimeout(err) && debugPort > 0 {
			slog.Warn("chrome startup timeout (Chrome 145+ regression), trying direct-launch fallback", "port", debugPort)
			time.Sleep(500 * time.Millisecond)
			return startChromeWithRemoteAllocator(parentCtx, cfg, debugPort, seededScript)
		}

		return nil, nil, fmt.Errorf("failed to connect to chrome: %w", err)
	}

	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return injectedScript(ctx, seededScript)
	})); err != nil {
		browserCancel()
		allocCancel()
		return nil, nil, fmt.Errorf("failed to inject stealth script: %w", err)
	}

	return browserCtx, func() {
		browserCancel()
		allocCancel()
	}, nil
}

func isStartupTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "context deadline exceeded")
}

func startChromeWithRemoteAllocator(parentCtx context.Context, cfg *config.RuntimeConfig, debugPort int, seededScript string) (context.Context, context.CancelFunc, error) {
	chromeBinary := cfg.ChromeBinary
	if chromeBinary == "" {
		chromeBinary = findChromeBinary()
	}
	if chromeBinary == "" {
		return nil, nil, fmt.Errorf("chrome/chromium not found: please install chrome or chromium, or set 'binary' in config.json")
	}

	args := BuildChromeArgs(cfg, debugPort)
	// #nosec G204 -- chromeBinary from user config or findChromeBinary() known system paths
	cmd := exec.Command(chromeBinary, args...)
	cmd.Stdout = newPrefixedLogWriter(os.Stdout, "chrome stdout")
	cmd.Stderr = newPrefixedLogWriter(os.Stderr, "chrome stderr")
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start chrome directly: %w", err)
	}

	wsURL, err := waitForChromeDevTools(debugPort, 30*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, nil, fmt.Errorf("chrome devtools not ready on port %d: %w", debugPort, err)
	}

	remoteAllocCtx, remoteAllocCancel := chromedp.NewRemoteAllocator(parentCtx, wsURL)
	browserCtx, browserCancel := chromedp.NewContext(remoteAllocCtx)

	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return injectedScript(ctx, seededScript)
	})); err != nil {
		browserCancel()
		remoteAllocCancel()
		_ = cmd.Process.Kill()
		return nil, nil, fmt.Errorf("failed to connect/inject via remote: %w", err)
	}

	return browserCtx, func() {
		browserCancel()
		remoteAllocCancel()
		_ = cmd.Process.Kill()
	}, nil
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

func DefaultChromeFlagArgs() []string {
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
		"--disable-automation",
		"--disable-blink-features=AutomationControlled",
	}
}

func appendChromeCompatibilityFlags(args []string) []string {
	if goruntime.GOOS == "linux" && os.Geteuid() == 0 {
		return append(args, "--no-sandbox")
	}
	return args
}

func BuildChromeArgs(cfg *config.RuntimeConfig, port int) []string {
	args := append([]string{fmt.Sprintf("--remote-debugging-port=%d", port)}, DefaultChromeFlagArgs()...)

	if len(cfg.ExtensionPaths) > 0 {
		joined := strings.Join(cfg.ExtensionPaths, ",")
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

	if cfg.ChromeExtraFlags != "" {
		args = append(args, strings.Fields(cfg.ChromeExtraFlags)...)
	}

	return appendChromeCompatibilityFlags(args)
}

const PopupGuardInitScript = `(function() {
  const mergeWindowFeatures = function(features) {
    const parts = typeof features === 'string'
      ? features.split(',').map(function(part) { return String(part).trim(); }).filter(Boolean)
      : [];
    const seen = new Set(parts.map(function(part) { return part.toLowerCase(); }));
    if (!seen.has('noopener')) {
      parts.push('noopener');
    }
    if (!seen.has('noreferrer')) {
      parts.push('noreferrer');
    }
    return parts.join(',');
  };

  try {
    const originalOpen = window.open;
    if (typeof originalOpen === 'function') {
      Object.defineProperty(window, 'open', {
        configurable: true,
        writable: true,
        value: function(url, target, features) {
          return originalOpen.call(window, url, target, mergeWindowFeatures(features));
        }
      });
    }
  } catch (_) {}

  try {
    Object.defineProperty(window, 'opener', {
      configurable: true,
      get: function() { return null; },
      set: function() { return true; }
    });
  } catch (_) {
    try {
      window.opener = null;
    } catch (_) {}
  }
})();`

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
	s := sizes[rand.Intn(len(sizes))]
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
