package doctor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"time"

	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
)

type probeResult struct {
	Port       int
	VersionURL string
}

var devtoolsRe = regexp.MustCompile(`DevTools listening on ws://[^:]+:(\d+)/`)

// launchAndProbe starts binary headless, waits for the DevTools banner,
// confirms /json/version responds, and tears the browser down before return.
func launchAndProbe(ctx context.Context, binary string, extraArgs []string, timeout time.Duration) (probeResult, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "pinchtab-doctor-*")
	if err != nil {
		return probeResult{}, fmt.Errorf("create user-data-dir: %w", err)
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

	cmd := exec.CommandContext(cctx, binary, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return probeResult{}, fmt.Errorf("attach stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return probeResult{}, fmt.Errorf("start browser: %w", err)
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
		return probeResult{}, fmt.Errorf("timed out waiting for DevTools banner: %w", cctx.Err())
	case err := <-procExitCh:
		procReaped = true
		if err == nil {
			return probeResult{}, errors.New("browser exited cleanly before exposing DevTools")
		}
		return probeResult{}, fmt.Errorf("browser exited before DevTools banner: %w", err)
	case err := <-errCh:
		return probeResult{}, err
	case port = <-portCh:
	}

	versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	probe, err := probeCDPVersionWithRetry(cctx, versionURL)
	if err != nil {
		return probeResult{Port: port, VersionURL: versionURL}, err
	}
	return probeResult{Port: port, VersionURL: probe.VersionURL}, nil
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

func probeCDPVersionWithRetry(ctx context.Context, url string) (bridgeruntime.CDPVersionProbe, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("deadline exceeded")
			}
			return bridgeruntime.CDPVersionProbe{}, fmt.Errorf("probe %s: %w", url, lastErr)
		}
		probe, err := bridgeruntime.ProbeCDPVersion(ctx, url, nil)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return probe, nil
	}
}

func checkCDPReachable(ctx context.Context, cfg *config.RuntimeConfig) CheckResult {
	bin := resolveBinary(cfg)
	if bin == "" {
		return CheckResult{Status: StatusSkip, Detail: "skipped — no browser.binary set (see cloakbrowser_present)"}
	}
	res, err := launchAndProbe(ctx, bin, nil, 10*time.Second)
	if err != nil {
		return CheckResult{Status: StatusFail, Detail: err.Error(), Err: err}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: fmt.Sprintf("/json/version OK on port %d", res.Port),
	}
}

func checkFingerprintFlagsAccepted(ctx context.Context, cfg *config.RuntimeConfig) CheckResult {
	bin := resolveBinary(cfg)
	if bin == "" {
		return CheckResult{Status: StatusSkip, Detail: "skipped — no browser.binary set (see cloakbrowser_present)"}
	}
	args := bridgeruntime.CloakBrowserFlagArgs(cfg)
	if len(args) == 0 {
		return CheckResult{
			Status: StatusSkip,
			Detail: "no cloak fingerprint flags configured",
		}
	}
	res, err := launchAndProbe(ctx, bin, args, 10*time.Second)
	if err != nil {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("flags rejected or browser crashed: %v", err),
			Err:    err,
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: fmt.Sprintf("flags accepted, /json/version OK on port %d", res.Port),
	}
}
