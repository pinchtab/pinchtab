package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/readiness"
	"github.com/pinchtab/pinchtab/internal/server"
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

	slog.Debug("server not running, starting automatically", "url", baseURL, "command", command)
	if err := start(); err != nil {
		slog.Error("failed to auto-start server", "err", err, "command", command)
		return fmt.Errorf("server at %s is not running and auto-start failed: %w", baseURL, err)
	}

	if !waitForServerWith(baseURL, token, timeout, healthy) {
		return fmt.Errorf("server did not become healthy at %s within %s", baseURL, timeout)
	}

	slog.Debug("server started successfully", "url", baseURL, "command", command)
	return nil
}

func isServerHealthy(baseURL, token string) bool {
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	status, _, reachable := server.ProbeHealth(baseURL+"/health", 3*time.Second, headers)
	return reachable && status < 500
}

func autoStartServer() error {
	stateDir := stateDirForConfig(loadConfig())
	binary, marker, err := prepareServerSpawn()
	if err != nil {
		return err
	}

	args := autoStartServerArgs(marker)
	pid, err := spawnDetachedChild(binary, args, nil)
	if err != nil {
		return err
	}

	// Track the auto-started server's PID so `pinchtab server stop` can reap
	// it. Best-effort: failing to write the PID file is logged but not fatal.
	if err := recordServerPID(stateDir, pid, binary, args, "", marker); err != nil {
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
	_, err := readiness.WaitUntil(context.Background(), timeout, 500*time.Millisecond,
		func() (struct{}, bool, error) { return struct{}{}, healthy(baseURL, token), nil })
	return err == nil
}
