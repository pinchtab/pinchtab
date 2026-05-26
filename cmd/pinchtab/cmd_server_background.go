package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
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
	Extensions []string
	Browser    string
}

var readProcessCommand = defaultReadProcessCommand

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
		cfg := loadConfig()

		if server.CheckPinchTabRunning(cfg.Port, cfg.Token) {
			fmt.Println("Stopping server...")
			if err := runServerStop(); err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, fmt.Sprintf("stop: %v", err)))
			}
		}

		fmt.Println("Starting server...")
		if err := runServerBackground(cfg, serverBackgroundOptions{}); err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
			os.Exit(1)
		}
	},
}

func runtimeStateDir() string {
	return filepath.Dir(config.DefaultConfigPath())
}

func serverPIDFilePath() string {
	return filepath.Join(runtimeStateDir(), "server.pid")
}

func serverLogFilePath() string {
	return filepath.Join(runtimeStateDir(), "server.log")
}

func runServerBackground(cfg *config.RuntimeConfig, opts serverBackgroundOptions) error {
	if info, ok := readServerPID(); ok {
		if processAlive(info.PID) {
			if err := verifyServerPIDInfo(info); err == nil {
				return fmt.Errorf("server already running (pid %d); stop with: pinchtab server stop", info.PID)
			} else {
				return fmt.Errorf("background PID file at %s points to a live process that cannot be verified: %w", serverPIDFilePath(), err)
			}
		}
		_ = os.Remove(serverPIDFilePath())
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%s", cfg.Port)
	if isUnauthenticatedPinchTabServerReady(baseURL) {
		return fmt.Errorf("server already running at %s; stop it before starting a background server", baseURL)
	}
	if portIsListening(baseURL) {
		return fmt.Errorf("port already in use at %s, but it is not a healthy PinchTab server for this config", baseURL)
	}

	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	marker, err := newBackgroundMarker()
	if err != nil {
		return fmt.Errorf("generate background marker: %w", err)
	}

	if err := os.MkdirAll(runtimeStateDir(), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	logF, err := os.OpenFile(serverLogFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	args := backgroundServerArgs(marker, opts)
	c := exec.Command(binary, args...) // #nosec G204 -- binary is our own executable
	c.Stdin = nil
	c.Stdout = logF
	c.Stderr = logF
	detachProcess(c)

	if err := c.Start(); err != nil {
		_ = logF.Close()
		return fmt.Errorf("spawn server: %w", err)
	}
	pid := c.Process.Pid
	if err := c.Process.Release(); err != nil {
		// non-fatal
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.MutedStyle, fmt.Sprintf("warn: release child: %v", err)))
	}
	_ = logF.Close()

	if err := writeServerPID(serverPIDInfo{
		PID:        pid,
		Executable: binary,
		Args:       append([]string(nil), args...),
		URL:        baseURL,
		Marker:     marker,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	if !waitForServerWith(baseURL, marker, backgroundStartTimeout, isBackgroundServerReady) {
		if !processAlive(pid) {
			_ = os.Remove(serverPIDFilePath())
		}
		return fmt.Errorf("server did not become healthy within %s; check logs at %s", backgroundStartTimeout, serverLogFilePath())
	}

	out := map[string]any{
		"pid":     pid,
		"url":     baseURL,
		"token":   cfg.Token,
		"logFile": serverLogFilePath(),
		"pidFile": serverPIDFilePath(),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func backgroundServerArgs(marker string, opts serverBackgroundOptions) []string {
	args := []string{"server", "--background-child", marker}
	if opts.Yolo {
		args = append(args, "-y")
	}
	if opts.Headed {
		args = append(args, "-H")
	}
	if opts.Verbose {
		args = append(args, "-v")
	}
	for _, ext := range opts.Extensions {
		args = append(args, "-e", ext)
	}
	if opts.Browser != "" {
		args = append(args, "--browser", opts.Browser)
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

func isPinchTabHealthReady(url, marker string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if marker != "" {
		req.Header.Set(backgroundHealthProbeHeader, marker)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var health struct {
		Status  string `json:"status"`
		Mode    string `json:"mode"`
		Version string `json:"version"`
		Marker  string `json:"marker"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
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
	info, ok := readServerPID()
	if !ok {
		return stopViaAPI()
	}
	pid := info.PID
	if !processAlive(pid) {
		_ = os.Remove(serverPIDFilePath())
		return stopViaAPI()
	}
	if err := verifyServerPIDInfo(info); err != nil {
		return err
	}
	if err := stopProcess(pid); err != nil {
		return fmt.Errorf("stop pid %d: %w", pid, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if processAlive(pid) {
		return fmt.Errorf("background server (pid %d) did not exit within 5s; leaving pid file at %s", pid, serverPIDFilePath())
	}
	_ = os.Remove(serverPIDFilePath())
	fmt.Printf("Stopped background server (pid %d)\n", pid)
	return nil
}

func newBackgroundMarker() (string, error) {
	var b [backgroundMarkerBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func writeServerPID(info serverPIDInfo) error {
	if err := os.MkdirAll(runtimeStateDir(), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pid info: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(serverPIDFilePath(), data, 0o600)
}

func readServerPID() (serverPIDInfo, bool) {
	data, err := os.ReadFile(serverPIDFilePath())
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
