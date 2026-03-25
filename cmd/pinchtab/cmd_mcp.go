package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP stdio server",
	Long:  "Start the Model Context Protocol stdio server and proxy browser actions to a running PinchTab instance.",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		runMCP(cfg)
	},
}

func init() {
	mcpCmd.GroupID = "primary"
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cfg *config.RuntimeConfig) {
	baseURL := resolveCLIBase(cfg)
	token := resolveCLIToken(cfg)

	if !isServerHealthy(baseURL, token) {
		slog.Info("server not running, starting automatically", "url", baseURL)
		if err := autoStartServer(); err != nil {
			slog.Error("failed to auto-start server", "err", err)
			fmt.Fprintf(os.Stderr, "mcp: server at %s is not running and auto-start failed: %v\n", baseURL, err)
			os.Exit(1)
		}
		if !waitForServer(baseURL, token, 30*time.Second) {
			fmt.Fprintf(os.Stderr, "mcp: server did not become healthy at %s within 30s\n", baseURL)
			os.Exit(1)
		}
		slog.Info("server started successfully")
	}

	mcp.Version = version

	if err := mcp.Serve(baseURL, token); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
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

	// Build args: forward --server flag if set, plus "server" subcommand
	args := []string{"server"}
	if serverURL != "" {
		args = []string{"--server", serverURL, "server"}
	}

	cmd := exec.Command(binary, args...) // #nosec G204 -- binary is our own executable from os.Executable(), args are hardcoded subcommands
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Detach from parent process group so the server survives if the
	// MCP process exits.
	detachProcess(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn server: %w", err)
	}

	// Release the child so it's not reaped when we exit.
	if err := cmd.Process.Release(); err != nil {
		slog.Warn("failed to release server process", "err", err)
	}

	return nil
}

func waitForServer(baseURL, token string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isServerHealthy(baseURL, token) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
