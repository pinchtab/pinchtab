package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

const ensureServerTimeout = 30 * time.Second

type serverStartFunc func() error
type serverHealthFunc func(baseURL, token string) bool

func ensureServerForCLI(cfg *config.RuntimeConfig, baseURL, token, command string) error {
	return ensureServerWithAutoStart(baseURL, token, command, canAutoStartServerForCLI(cfg, baseURL), autoStartServer, isServerHealthy, ensureServerTimeout)
}

func ensureServerWith(baseURL, token, command string, start serverStartFunc, healthy serverHealthFunc, timeout time.Duration) error {
	return ensureServerWithAutoStart(baseURL, token, command, true, start, healthy, timeout)
}

func ensureServerWithAutoStart(baseURL, token, command string, allowAutoStart bool, start serverStartFunc, healthy serverHealthFunc, timeout time.Duration) error {
	if healthy(baseURL, token) {
		return nil
	}

	if !allowAutoStart {
		return fmt.Errorf("server at %s is not running; auto-start is only supported for the default local server", baseURL)
	}

	slog.Info("server not running, starting automatically", "url", baseURL, "command", command)
	if err := start(); err != nil {
		slog.Error("failed to auto-start server", "err", err, "command", command)
		return fmt.Errorf("server at %s is not running and auto-start failed: %w", baseURL, err)
	}

	if !waitForServerWith(baseURL, token, timeout, healthy) {
		return fmt.Errorf("server did not become healthy at %s within %s", baseURL, timeout)
	}

	slog.Info("server started successfully", "url", baseURL, "command", command)
	return nil
}

func isServerHealthy(baseURL, token string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode < 500
}

func autoStartServer() error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	marker, err := newBackgroundMarker()
	if err != nil {
		return fmt.Errorf("generate background marker: %w", err)
	}

	args := autoStartServerArgs(marker)
	cmd := exec.Command(binary, args...) // #nosec G204 -- binary is our own executable from os.Executable(), args are hardcoded subcommands
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	detachProcess(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn server: %w", err)
	}

	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		slog.Warn("failed to release server process", "err", err)
	}

	// Track the auto-started server's PID so `pinchtab server stop` can reap
	// it. Best-effort: failing to write the PID file is logged but not fatal.
	if err := writeServerPID(serverPIDInfo{
		PID:        pid,
		Executable: binary,
		Args:       append([]string(nil), args...),
		Marker:     marker,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		slog.Warn("failed to write server pid file", "err", err)
	}

	return nil
}

func autoStartServerArgs(marker ...string) []string {
	args := []string{"server"}
	if len(marker) > 0 && strings.TrimSpace(marker[0]) != "" {
		args = append(args, "--background-child", marker[0])
	}
	return args
}

func waitForServer(baseURL, token string, timeout time.Duration) bool {
	return waitForServerWith(baseURL, token, timeout, isServerHealthy)
}

func waitForServerWith(baseURL, token string, timeout time.Duration, healthy serverHealthFunc) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if healthy(baseURL, token) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
