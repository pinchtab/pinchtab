package main

import (
	"context"
	"errors"
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
type serverProbeFunc func(baseURL, token string) server.HealthProbe

func ensureServerForCLI(cfg *config.RuntimeConfig, baseURL, token, command string) error {
	return ensureServerWithDaemonRecovery(
		baseURL, token, command,
		canAutoStartServerForCLI(cfg, baseURL),
		autoStartServer, probeServerHealth,
		detachedDaemonOwnership, startInstalledDaemon,
		ensureServerTimeout,
	)
}

// startInstalledDaemon asks the installed service manager to recover the
// daemon. It is intentionally separate from autoStartServer: only the service
// manager may create a server when it owns the lifecycle.
func startInstalledDaemon() (string, error) {
	manager, err := daemonCurrentManager()
	if err != nil {
		return "", fmt.Errorf("access background service manager: %w", err)
	}
	return manager.Start()
}

// ensureServerWithDaemonRecovery preserves CLI cold-start ergonomics without
// allowing a detached child to compete with an installed daemon. A healthy
// daemon needs no action; an unreachable installed daemon is started through
// its manager and given the normal bounded readiness wait. A reachable-but-
// unhealthy listener is never restarted implicitly because it may be a rogue
// child holding the daemon port.
func ensureServerWithDaemonRecovery(baseURL, token, command string, allowAutoStart bool, startDetached serverStartFunc, probe serverProbeFunc, ownership func() (bool, error), startDaemon func() (string, error), timeout time.Duration) error {
	initial := probe(baseURL, token)
	if serverProbeHealthy(initial) {
		return nil
	}
	if !allowAutoStart {
		if initial.Reachable {
			return fmt.Errorf("server at %s is running but unhealthy: %s", baseURL, unhealthyServerDetail(initial))
		}
		return fmt.Errorf("server at %s is not running; auto-start is only supported for the default local server", baseURL)
	}

	installed, err := ownership()
	if err != nil {
		return fmt.Errorf("cannot determine background-service ownership; refusing automatic start: %w", err)
	}
	if installed {
		if initial.Reachable {
			return fmt.Errorf("server at %s is running but unhealthy: %s; an installed PinchTab daemon owns lifecycle, so refusing detached auto-start. Check `pinchtab daemon status` and the port owner", baseURL, unhealthyServerDetail(initial))
		}
		message, err := startDaemon()
		if err != nil {
			return fmt.Errorf("installed PinchTab daemon owns lifecycle, but could not start it: %w; refusing detached auto-start", err)
		}
		ready := func(url, tok string) bool { return serverProbeHealthy(probe(url, tok)) }
		if !waitForServerWith(baseURL, token, timeout, ready) {
			return fmt.Errorf("installed PinchTab daemon was asked to start (%s), but server did not become healthy at %s within %s; a stale or rogue listener may own the port. Run `pinchtab daemon status` and inspect the port owner; refusing detached auto-start", message, baseURL, timeout)
		}
		return nil
	}

	return ensureServerWithAutoStart(baseURL, token, command, true, startDetached, probe, timeout)
}

func ensureServerWithAutoStart(baseURL, token, command string, allowAutoStart bool, start serverStartFunc, probe serverProbeFunc, timeout time.Duration) error {
	initial := probe(baseURL, token)
	if serverProbeHealthy(initial) {
		return nil
	}

	// Reachable but unhealthy: the server is up and answered this very probe, so
	// starting a second one cannot fix whatever it reported. Quote its reason
	// instead of waiting out the readiness timeout.
	if initial.Reachable {
		return fmt.Errorf("server at %s is running but unhealthy: %s", baseURL, unhealthyServerDetail(initial))
	}

	if !allowAutoStart {
		return fmt.Errorf("server at %s is not running; auto-start is only supported for the default local server", baseURL)
	}

	slog.Debug("server not running, starting automatically", "url", baseURL, "command", command)
	if err := start(); err != nil {
		slog.Error("failed to auto-start server", "err", err, "command", command)
		return fmt.Errorf("server at %s is not running and auto-start failed: %w", baseURL, err)
	}

	// The readiness wait keeps the bool semantics: a freshly spawned server
	// answers 503 while its browser comes up, which is not-ready-yet, not a
	// reason to abort.
	ready := func(url, tok string) bool { return serverProbeHealthy(probe(url, tok)) }
	if !waitForServerWith(baseURL, token, timeout, ready) {
		return fmt.Errorf("server did not become healthy at %s within %s", baseURL, timeout)
	}

	slog.Debug("server started successfully", "url", baseURL, "command", command)
	return nil
}

func unhealthyServerDetail(probe server.HealthProbe) string {
	if reason := probe.Reason(); reason != "" {
		return reason
	}
	return fmt.Sprintf("HTTP %d", probe.StatusCode)
}

func probeServerHealth(baseURL, token string) server.HealthProbe {
	return server.ProbeHealthWithToken(baseURL+"/health", 3*time.Second, token)
}

// serverProbeHealthy is the CLI ensure path's own readiness rule: anything the
// server answers below 500 means it is up enough to take the command.
func serverProbeHealthy(probe server.HealthProbe) bool {
	return probe.Reachable && probe.StatusCode < 500
}

func autoStartServer() error {
	if err := requireDetachedServerOwnership("automatic start", false); err != nil {
		return err
	}
	return autoStartServerIn(stateDirForConfig(loadConfig()))
}

func autoStartServerIn(stateDir string) error {
	_, _, err := spawnRecordedServer(stateDir, func(marker string) []string { return autoStartServerArgs(marker) })
	if errors.Is(err, errServerPIDNotRecorded) {
		slog.Warn("failed to write server pid file", "err", err)
		return nil
	}
	return err
}

func autoStartServerArgs(marker ...string) []string {
	args := []string{"server"}
	if len(marker) > 0 && strings.TrimSpace(marker[0]) != "" {
		args = append(args, backgroundChildFlag, marker[0])
	}
	return args
}

func waitForServerWith(baseURL, token string, timeout time.Duration, healthy serverHealthFunc) bool {
	_, err := readiness.WaitUntil(context.Background(), timeout, 500*time.Millisecond,
		func() (struct{}, bool, error) { return struct{}{}, healthy(baseURL, token), nil })
	return err == nil
}
