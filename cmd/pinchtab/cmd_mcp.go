package main

import (
	"fmt"
	"os"

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

	// Token from config, env var overrides
	token := resolveCLIToken(cfg)

	mcp.Version = version

	if err := mcp.Serve(baseURL, token); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
