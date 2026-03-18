package main

import (
	"fmt"
	"os"
	"strings"

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
	// Default: http://127.0.0.1:{port}
	port := cfg.Port
	if port == "" {
		port = "9867"
	}
	baseURL := "http://127.0.0.1:" + port

	// --server flag overrides
	if serverURL != "" {
		baseURL = strings.TrimRight(serverURL, "/")
	}

	// Token from config, env var overrides
	token := cfg.Token
	if envToken := os.Getenv("PINCHTAB_TOKEN"); envToken != "" {
		token = envToken
	}

	mcp.Version = version

	if err := mcp.Serve(baseURL, token); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
