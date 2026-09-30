package chrome

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/readiness"
)

type CDPProbeResult struct {
	Port         int
	VersionURL   string
	WebSocketURL string
}

var devtoolsRe = regexp.MustCompile(`DevTools listening on ws://[^:]+:(\d+)/`)

// LaunchAndProbe starts binary headless, waits for the DevTools banner,
// confirms /json/version responds, and tears the browser down before return.
func LaunchAndProbe(ctx context.Context, binary string, extraArgs []string, timeout time.Duration) (CDPProbeResult, error) {
	return launchAndProbe(ctx, binary, extraArgs, timeout, nil)
}

// LaunchAndEvaluate performs the launch probe and evaluates one expression
// before tearing the browser down. It lets provider diagnostics verify behavior
// that a version string or a successfully opened CDP port cannot prove.
func LaunchAndEvaluate(ctx context.Context, binary string, extraArgs []string, timeout time.Duration, expression string, value any) (CDPProbeResult, error) {
	return launchAndProbe(ctx, binary, extraArgs, timeout, func(ctx context.Context, result CDPProbeResult) error {
		allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, result.WebSocketURL, chromedp.NoModifyURL)
		defer allocCancel()
		browserCtx, browserCancel := chromedp.NewContext(allocCtx)
		defer browserCancel()
		if err := chromedp.Run(browserCtx, chromedp.Evaluate(expression, value)); err != nil {
			return fmt.Errorf("evaluate %q: %w", expression, err)
		}
		return nil
	})
}

func launchAndProbe(ctx context.Context, binary string, extraArgs []string, timeout time.Duration, inspect func(context.Context, CDPProbeResult) error) (CDPProbeResult, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "pinchtab-doctor-*")
	if err != nil {
		return CDPProbeResult{}, fmt.Errorf("create user-data-dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	args := []string{
		"--headless=new",
		"--remote-debugging-port=0",
		"--user-data-dir=" + tmpDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-gpu",
	}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(cctx, binary, args...) // #nosec G204 -- binary is a browser path from config or provider discovery, not user input
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return CDPProbeResult{}, fmt.Errorf("attach stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return CDPProbeResult{}, fmt.Errorf("start browser: %w", err)
	}

	portCh := make(chan int, 1)
	errCh := make(chan error, 1)
	go scrapeDevtoolsPort(stderr, portCh, errCh)

	// Race: process exit, ctx timeout, port discovered.
	procExitCh := make(chan error, 1)
	go func() { procExitCh <- cmd.Wait() }()
	procReaped := false

	// Ensure the subprocess is reaped on return so we don't leak zombies even
	// when context cancellation races the parse loop. cmd.Wait is owned by the
	// goroutine above; this defer only kills and drains that result.
	defer func() {
		if procReaped {
			return
		}
		_ = cmd.Process.Kill()
		select {
		case <-procExitCh:
		case <-time.After(2 * time.Second):
		}
	}()

	var port int
	select {
	case <-cctx.Done():
		return CDPProbeResult{}, fmt.Errorf("timed out waiting for DevTools banner: %w", cctx.Err())
	case err := <-procExitCh:
		procReaped = true
		if err == nil {
			return CDPProbeResult{}, errors.New("browser exited cleanly before exposing DevTools")
		}
		return CDPProbeResult{}, fmt.Errorf("browser exited before DevTools banner: %w", err)
	case err := <-errCh:
		return CDPProbeResult{}, err
	case port = <-portCh:
	}

	versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	webSocketURL, err := probeCDPVersionWithRetry(cctx, versionURL)
	result := CDPProbeResult{Port: port, VersionURL: versionURL, WebSocketURL: webSocketURL}
	if err != nil {
		return result, err
	}
	if inspect != nil {
		if err := inspect(cctx, result); err != nil {
			return result, fmt.Errorf("inspect launched browser: %w", err)
		}
	}
	return result, nil
}

func scrapeDevtoolsPort(r io.ReadCloser, portCh chan<- int, errCh chan<- error) {
	defer func() { _ = r.Close() }()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		m := devtoolsRe.FindStringSubmatch(line)
		if len(m) == 2 {
			var port int
			_, err := fmt.Sscanf(m[1], "%d", &port)
			if err != nil {
				errCh <- fmt.Errorf("parse DevTools port %q: %w", m[1], err)
				return
			}
			portCh <- port
			// Drain remaining stderr so the child doesn't block on a full pipe.
			for scanner.Scan() {
			}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		errCh <- fmt.Errorf("read stderr: %w", err)
		return
	}
	errCh <- errors.New("stderr closed before DevTools banner appeared")
}

const cdpProbeInterval = 100 * time.Millisecond

func probeCDPVersionWithRetry(ctx context.Context, url string) (string, error) {
	timeout := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	lastErr := errors.New("deadline exceeded")
	wsURL, err := readiness.WaitUntil(ctx, timeout, cdpProbeInterval, func() (string, bool, error) {
		wsURL, probeErr := readiness.ProbeDevToolsWebSocketURL(ctx, client, url)
		if probeErr != nil {
			lastErr = probeErr
			return "", false, nil
		}
		return wsURL, true, nil
	})
	if errors.Is(err, readiness.ErrNotReady) {
		err = lastErr
	}
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", url, err)
	}
	return wsURL, nil
}
