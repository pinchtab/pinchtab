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
)

// CDPProbeResult holds the outcome of a successful LaunchAndProbe call.
type CDPProbeResult struct {
	Port       int
	VersionURL string
}

var devtoolsRe = regexp.MustCompile(`DevTools listening on ws://[^:]+:(\d+)/`)

// LaunchAndProbe starts binary headless, waits for the DevTools banner,
// confirms /json/version responds, and tears the browser down before return.
func LaunchAndProbe(ctx context.Context, binary string, extraArgs []string, timeout time.Duration) (CDPProbeResult, error) {
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

	cmd := exec.CommandContext(cctx, binary, args...)
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
	if err := probeCDPVersionWithRetry(cctx, versionURL); err != nil {
		return CDPProbeResult{Port: port, VersionURL: versionURL}, err
	}
	return CDPProbeResult{Port: port, VersionURL: versionURL}, nil
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

// probeCDPVersionWithRetry polls the /json/version endpoint until it responds
// HTTP 200 or the context deadline is reached.
func probeCDPVersionWithRetry(ctx context.Context, url string) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("deadline exceeded")
			}
			return fmt.Errorf("probe %s: %w", url, lastErr)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("probe %s: %w", url, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		time.Sleep(100 * time.Millisecond)
	}
}
