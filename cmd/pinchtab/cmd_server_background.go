package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pinchtab/pinchtab/internal/readiness"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/daemon"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

const backgroundStartTimeout = 30 * time.Second
const backgroundMarkerBytes = 16
const backgroundHealthProbeHeader = "PinchTab-Background-Marker"

type serverPIDInfo struct {
	PID        int      `json:"pid"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	URL        string   `json:"url"`
	Marker     string   `json:"marker"`
	StartedAt  string   `json:"startedAt"`
}

type serverBackgroundOptions struct {
	Yolo       bool
	Headed     bool
	Verbose    bool
	LogLevel   string
	Extensions []string
	Browser    string
	Bind       string
	Port       string
}

var readProcessCommand = defaultReadProcessCommand
var daemonInstallationStatus = daemon.InstallationStatus

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running server",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runServerStop(); err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
			os.Exit(1)
		}
	},
}

var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the running server (stop + start in background)",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runServerRestart(loadConfig()); err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
			os.Exit(1)
		}
	},
}

func detachedDaemonOwnership() (bool, error) {
	installed, err := daemonInstallationStatus()
	if err != nil {
		if errors.Is(err, daemon.ErrUnsupportedOS) {
			return false, nil
		}
		return false, err
	}
	return installed, nil
}

func requireDetachedServerOwnership(action string, addressChanged bool) error {
	if addressChanged {
		return nil
	}
	installed, err := detachedDaemonOwnership()
	if err != nil {
		return fmt.Errorf("cannot determine background-service ownership; refusing %s: %w", action, err)
	}
	if installed {
		return fmt.Errorf("background service is installed; use `pinchtab daemon start` so one service manager owns the server, or pass --bind/--port with a different address to run a separate detached server")
	}
	return nil
}

func runServerRestart(cfg *config.RuntimeConfig) error {
	installed, err := detachedDaemonOwnership()
	if err != nil {
		return fmt.Errorf("cannot determine background-service ownership; refusing restart: %w", err)
	}
	if installed {
		return fmt.Errorf("background service is installed; use `pinchtab daemon restart` so one service manager owns the server")
	}
	if server.CheckPinchTabRunning(cfg.Port, cfg.Token) {
		fmt.Println("Stopping server...")
		if err := runServerStop(); err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, fmt.Sprintf("stop: %v", err)))
		}
	}
	fmt.Println("Starting server...")
	return runServerBackground(cfg, serverBackgroundOptions{}, false)
}

func stateDirForConfig(cfg *config.RuntimeConfig) string {
	if cfg != nil && strings.TrimSpace(cfg.StateDir) != "" {
		return strings.TrimSpace(cfg.StateDir)
	}
	return filepath.Dir(config.DefaultConfigPath())
}

func serverPIDFilePath(stateDir string) string {
	return filepath.Join(stateDir, "server.pid")
}

func serverLogFilePath(stateDir string) string {
	return filepath.Join(stateDir, "server.log")
}

// backgroundChildFlag marks a server the CLI spawned detached. The spawn site and
// the "is this server a background child" question have to spell it the same way, so
// they share this rather than repeating the literal.
const (
	backgroundChildFlagName = "background-child"
	backgroundChildFlag     = "--" + backgroundChildFlagName
)

// serverLogWhere answers the question an operator asks after a 5xx: which file is
// this server writing to. The three launch paths land in three different places, and
// a state dir usually holds a server.log from whichever one ran last — so the answer
// has to name the LIVE destination and disown a file nothing is writing, rather than
// let a stale path pass for a log.
type serverLogWhere struct {
	// Destination is the live log sink: a path, or a description when the logs go
	// to a terminal rather than a file.
	Destination string
	// StalePath is a server.log that exists while something else is the live sink.
	StalePath string
}

// resolveServerLogWhere is pure so the branch table is testable without a daemon, a
// state dir or a running server; the caller does the I/O and hands in the facts.
func resolveServerLogWhere(stateDir, daemonLogPath string, daemonInstalled, backgroundChild, running, serverLogExists bool) serverLogWhere {
	backgroundLog := serverLogFilePath(stateDir)
	where := serverLogWhere{}

	switch {
	case daemonInstalled:
		where.Destination = daemonLogPath
	case running && backgroundChild:
		where.Destination = backgroundLog
	case running:
		where.Destination = "stdout/stderr of the terminal running `pinchtab server`"
	default:
		where.Destination = "no server running"
	}

	if serverLogExists && where.Destination != backgroundLog {
		where.StalePath = backgroundLog
	}
	return where
}

// serverLogWhereForConfig gathers the facts resolveServerLogWhere needs. A daemon
// environment that cannot be resolved is reported as "no daemon" rather than as an
// error: the banner is a hint surface, and a missing home directory must not stop it
// from naming the other sinks.
func serverLogWhereForConfig(cfg *config.RuntimeConfig, running bool) serverLogWhere {
	stateDir := stateDirForConfig(cfg)
	daemonInstalled, err := detachedDaemonOwnership()
	if err != nil {
		daemonInstalled = false
	}
	daemonLogPath := ""
	if daemonInstalled {
		if path, pathErr := daemon.StderrLogPath(); pathErr == nil {
			daemonLogPath = path
		} else {
			daemonLogPath = "pinchtab daemon logs"
		}
	}

	backgroundChild := false
	if info, ok := readServerPID(stateDir); ok && processAlive(info.PID) {
		backgroundChild = slices.Contains(info.Args, backgroundChildFlag)
	}
	_, statErr := os.Stat(serverLogFilePath(stateDir))

	return resolveServerLogWhere(stateDir, daemonLogPath, daemonInstalled, backgroundChild, running, statErr == nil)
}

func prepareServerSpawn() (binary, marker string, err error) {
	binary, err = os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve executable: %w", err)
	}
	marker, err = newBackgroundMarker()
	if err != nil {
		return "", "", fmt.Errorf("generate background marker: %w", err)
	}
	return binary, marker, nil
}

// spawnDetachedChild starts binary+args detached and returns the child PID; the
// handle is released so the child outlives this process. out wires stdout/stderr
// (nil → discard) — never assign a typed-nil *os.File to the exec.Cmd writer
// fields, as that connects them to a nil file rather than /dev/null.
func spawnDetachedChild(binary string, args []string, out *os.File) (int, error) {
	c := exec.Command(binary, args...) // #nosec G204 -- binary is our own executable
	c.Stdin = nil
	if out != nil {
		c.Stdout = out
		c.Stderr = out
	}
	detachProcess(c)
	if err := c.Start(); err != nil {
		return 0, fmt.Errorf("spawn server: %w", err)
	}
	pid := c.Process.Pid
	if err := c.Process.Release(); err != nil {
		slog.Warn("failed to release server process", "err", err)
	}
	return pid, nil
}

// spawnDetachedServer starts a detached server whose stdout/stderr land in the state
// dir's server.log. Every detached spawn path goes through here: the banner names that
// file as the live sink from the --background-child flag in the recorded argv alone, so
// a second spawn site that passed its own writer (or none) would make the banner vouch
// for a file nothing writes — which is the dead end this card exists to remove.
func spawnDetachedServer(stateDir, binary string, args []string) (int, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return 0, fmt.Errorf("create state dir: %w", err)
	}
	logF, err := os.OpenFile(serverLogFilePath(stateDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = logF.Close() }()
	return spawnDetachedChild(binary, args, logF)
}

func runServerBackground(cfg *config.RuntimeConfig, opts serverBackgroundOptions, addressChanged bool) error {
	if err := requireDetachedServerOwnership("background start", addressChanged); err != nil {
		return err
	}

	stateDir := stateDirForConfig(cfg)
	if info, ok := readServerPID(stateDir); ok {
		if processAlive(info.PID) {
			if err := verifyServerPIDInfo(info); err == nil {
				return fmt.Errorf("server already running (pid %d); stop with: pinchtab server stop", info.PID)
			} else {
				return fmt.Errorf("background PID file at %s points to a live process that cannot be verified: %w", serverPIDFilePath(stateDir), err)
			}
		}
		_ = os.Remove(serverPIDFilePath(stateDir))
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%s", cfg.Port)
	if err := portBusyError(baseURL, config.ConfigFilePath()); err != nil {
		return err
	}

	binary, marker, err := prepareServerSpawn()
	if err != nil {
		return err
	}

	args := backgroundServerArgs(marker, opts)
	pid, err := spawnDetachedServer(stateDir, binary, args)
	if err != nil {
		return err
	}

	if err := recordServerPID(stateDir, pid, binary, args, baseURL, marker); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	if !waitForServerWith(baseURL, marker, backgroundStartTimeout, isBackgroundServerReady) {
		if !processAlive(pid) {
			_ = os.Remove(serverPIDFilePath(stateDir))
		}
		return fmt.Errorf("server did not become healthy within %s; check logs at %s", backgroundStartTimeout, serverLogFilePath(stateDir))
	}

	out := map[string]any{
		"pid":     pid,
		"url":     baseURL,
		"token":   cfg.Token,
		"logFile": serverLogFilePath(stateDir),
		"pidFile": serverPIDFilePath(stateDir),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func backgroundServerArgs(marker string, opts serverBackgroundOptions) []string {
	args := []string{"server", backgroundChildFlag, marker}
	if opts.Yolo {
		args = append(args, "-y")
	}
	if opts.Headed {
		args = append(args, "-H")
	}
	if opts.Verbose {
		args = append(args, "-v")
	}
	if opts.LogLevel != "" {
		args = append(args, "--log-level", opts.LogLevel)
	}
	for _, ext := range opts.Extensions {
		args = append(args, "-e", ext)
	}
	if opts.Browser != "" {
		args = append(args, "--browser", opts.Browser)
	}
	// The parent already applied these to cfg and waits on the resulting URL, so
	// the detached child must bind the same address, not the config default.
	if opts.Bind != "" {
		args = append(args, "--bind", opts.Bind)
	}
	if opts.Port != "" {
		args = append(args, "--port", opts.Port)
	}
	return args
}

func isBackgroundServerReady(baseURL, marker string) bool {
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return false
	}
	return isPinchTabHealthReady(baseURL+"/health/background", marker)
}

func isUnauthenticatedPinchTabServerReady(baseURL string) bool {
	return isPinchTabHealthReady(baseURL+"/health", "")
}

// portBusyError classifies whatever holds the port and returns an
// actionable error, or nil when the port is free: a ready PinchTab server,
// a PinchTab server running with a different config/token (its /health
// answers with the PinchTab auth-error shape), or a foreign process.
func portBusyError(baseURL, configPath string) error {
	if isUnauthenticatedPinchTabServerReady(baseURL) {
		return fmt.Errorf("server already running at %s; stop it with: pinchtab server stop", baseURL)
	}
	if isAuthenticatedPinchTabServer(baseURL) {
		return fmt.Errorf("a PinchTab server (different config/token) is already running at %s; stop it with `pinchtab server stop` (or from its own checkout), or change \"port\" in %s", baseURL, configPath)
	}
	if portIsListening(baseURL) {
		return fmt.Errorf("port already in use at %s by a process that is not a PinchTab server; stop that process or change \"port\" in %s", baseURL, configPath)
	}
	return nil
}

// isAuthenticatedPinchTabServer reports whether /health answered with a
// PinchTab-shaped auth error, i.e. the occupant is a PinchTab server whose
// token this config does not hold.
func isAuthenticatedPinchTabServer(baseURL string) bool {
	status, body, reachable := server.ProbeHealth(baseURL+"/health", 3*time.Second, nil)
	if !reachable {
		return false
	}
	return isPinchTabAuthError(status, body)
}

func isPinchTabAuthError(status int, body []byte) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return resp.Error != "" && resp.Code != ""
}

func isPinchTabHealthReady(url, marker string) bool {
	var headers map[string]string
	if marker != "" {
		headers = map[string]string{backgroundHealthProbeHeader: marker}
	}
	status, body, reachable := server.ProbeHealth(url, 3*time.Second, headers)
	if !reachable || status != http.StatusOK {
		return false
	}

	var health struct {
		Status  string `json:"status"`
		Mode    string `json:"mode"`
		Version string `json:"version"`
		Marker  string `json:"marker"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return false
	}
	if health.Status != "ok" || health.Mode != "dashboard" || strings.TrimSpace(health.Version) == "" {
		return false
	}
	return marker == "" || health.Marker == marker
}

func stopViaAPI() error {
	cfg := loadConfig()
	if !server.CheckPinchTabRunning(cfg.Port, cfg.Token) {
		return fmt.Errorf("no server running on port %s", cfg.Port)
	}
	if err := server.ShutdownServer(cfg.Port, cfg.Token); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	fmt.Printf("Stopped server on port %s\n", cfg.Port)
	return nil
}

func runServerStop() error {
	cfg := loadConfig()
	stateDir := stateDirForConfig(cfg)
	info, ok := readServerPID(stateDir)
	if !ok {
		return stopViaAPI()
	}
	pid := info.PID
	if !processAlive(pid) {
		_ = os.Remove(serverPIDFilePath(stateDir))
		return stopViaAPI()
	}
	if err := verifyServerPIDInfo(info); err != nil {
		return err
	}
	if err := stopProcess(pid); err != nil {
		return fmt.Errorf("stop pid %d: %w", pid, err)
	}

	grace := gracefulStopTimeout(cfg)
	if waitForExit(pid, grace) {
		_ = os.Remove(serverPIDFilePath(stateDir))
		fmt.Printf("Stopped background server (pid %d)\n", pid)
		return nil
	}

	_ = forceKillProcess(pid)
	if waitForExit(pid, 2*time.Second) {
		_ = os.Remove(serverPIDFilePath(stateDir))
		fmt.Printf("Force-stopped background server (pid %d) after %s grace\n", pid, grace)
		return nil
	}
	return fmt.Errorf("background server (pid %d) did not exit after SIGTERM+SIGKILL; pid file left at %s", pid, serverPIDFilePath(stateDir))
}

func gracefulStopTimeout(cfg *config.RuntimeConfig) time.Duration {
	d := 10 * time.Second
	if cfg != nil && cfg.ShutdownTimeout > 0 {
		d = cfg.ShutdownTimeout
	}
	return d + 2*time.Second
}

func waitForExit(pid int, timeout time.Duration) bool {
	if _, err := readiness.WaitUntil(context.Background(), timeout, 200*time.Millisecond, func() (struct{}, bool, error) {
		return struct{}{}, !processAlive(pid), nil
	}); err == nil {
		return true
	}
	return !processAlive(pid)
}

func newBackgroundMarker() (string, error) {
	var b [backgroundMarkerBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func writeServerPID(stateDir string, info serverPIDInfo) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pid info: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(serverPIDFilePath(stateDir), data, 0o600)
}

// recordServerPID writes the PID record for a freshly spawned server child,
// stamping the start time. Shared by the foreground auto-start and background
// server paths (url is empty for the URL-less auto-start record); each caller
// keeps its own write-error policy (best-effort vs fatal).
func recordServerPID(stateDir string, pid int, binary string, args []string, url, marker string) error {
	return writeServerPID(stateDir, serverPIDInfo{
		PID:        pid,
		Executable: binary,
		Args:       append([]string(nil), args...),
		URL:        url,
		Marker:     marker,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func readServerPID(stateDir string) (serverPIDInfo, bool) {
	data, err := os.ReadFile(serverPIDFilePath(stateDir))
	if err != nil {
		return serverPIDInfo{}, false
	}
	var info serverPIDInfo
	if err := json.Unmarshal(data, &info); err == nil && info.PID > 0 {
		return info, true
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return serverPIDInfo{}, false
	}
	return serverPIDInfo{PID: pid}, true
}

func verifyServerPIDInfo(info serverPIDInfo) error {
	if info.PID <= 0 {
		return fmt.Errorf("invalid pid file: missing pid")
	}
	if strings.TrimSpace(info.Executable) == "" || strings.TrimSpace(info.Marker) == "" {
		return fmt.Errorf("pid file lacks verifiable background metadata; refusing to stop pid %d", info.PID)
	}
	command, err := readProcessCommand(info.PID)
	if err != nil {
		return fmt.Errorf("read process command for pid %d: %w", info.PID, err)
	}
	if !serverPIDCommandMatches(command, info) {
		return fmt.Errorf("pid %d does not match background server metadata; refusing to stop", info.PID)
	}
	return nil
}

func serverPIDCommandMatches(command string, info serverPIDInfo) bool {
	command = strings.TrimSpace(command)
	executable := strings.TrimSpace(info.Executable)
	if command == "" || executable == "" || info.Marker == "" {
		return false
	}
	if command != executable && !strings.HasPrefix(command, executable+" ") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(command, executable))
	fields := strings.Fields(rest)
	if len(fields) == 0 || fields[0] != "server" {
		return false
	}
	return strings.Contains(command, info.Marker)
}

func defaultReadProcessCommand(pid int) (string, error) {
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil && len(data) > 0 {
		parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		return strings.TrimSpace(strings.Join(parts, " ")), nil
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output() // #nosec G204 -- pid is an int argument to a static ps command, no shell expansion
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
