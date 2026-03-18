package main

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "pinchtab",
	Short: "PinchTab - Browser control for AI agents",
	Long: `PinchTab provides a lightweight, API-driven way for AI agents to control
browsers, manage tabs, and perform interactive tasks.`,
	Example: `  pinchtab server
  pinchtab nav https://pinchtab.com`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if security wizard needs to run
		maybeRunWizard()
		if isInteractiveTerminal() {
			cfg := loadLocalConfig()
			cli.PrintStartupBanner(cfg, cli.StartupBannerOptions{
				Mode:         "menu",
				ListenStatus: menuListenStatus(cfg),
			})

			picked, err := promptSelect("Main Menu", []menuOption{
				{label: "Start server", value: "server"},
				{label: "Daemon", value: "daemon"},
				{label: "Start bridge", value: "bridge"},
				{label: "Start MCP server", value: "mcp"},
				{label: "Config", value: "config"},
				{label: "Security", value: "security"},
				{label: "Help", value: "help"},
				{label: "Exit", value: "exit"},
			})

			if err != nil || picked == "exit" || picked == "" {
				return
			}

			switch picked {
			case "server":
				server.RunDashboard(loadConfig(), version)
			case "daemon":
				handleDaemonCommand("")
			case "bridge":
				server.RunBridgeServer(loadConfig())
			case "mcp":
				runMCP(loadConfig())
			case "config":
				handleConfigOverview(cfg)
			case "security":
				handleSecurityCommand(cfg)
			case "help":
				_ = cmd.Help()
			}
			return
		}

		// Fallback for non-interactive: start the server
		server.RunDashboard(loadConfig(), version)
	},
}

// maybeRunWizard checks if the security wizard should run and triggers it.
func maybeRunWizard() {
	fileCfg, configPath, err := config.LoadFileConfig()
	if err != nil || configPath == "" {
		return // No config file context — skip wizard
	}

	if !config.NeedsWizard(fileCfg) {
		return
	}

	isNew := config.IsFirstRun(fileCfg)
	runSecurityWizard(fileCfg, configPath, isNew)
}

func menuListenStatus(cfg *config.RuntimeConfig) string {
	dashPort := cfg.Port
	if dashPort == "" {
		dashPort = "9870"
	}
	if server.CheckPinchTabRunning(dashPort, cfg.Token) {
		return "running"
	}
	return "stopped"
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// serverURL is the global --server flag for CLI commands
var serverURL string

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("pinchtab {{.Version}}\n")

	// Global flags
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "PinchTab server URL (default: http://127.0.0.1:{port})")

	// Grouping commands
	primaryGroup := &cobra.Group{ID: "primary", Title: "Primary Commands"}
	browserGroup := &cobra.Group{ID: "browser", Title: "Browser Control"}
	mgmtGroup := &cobra.Group{ID: "management", Title: "Profiles and Instances"}
	configGroup := &cobra.Group{ID: "config", Title: "Configuration & Setup"}

	rootCmd.AddGroup(primaryGroup, browserGroup, mgmtGroup, configGroup)

	cli.SetupUsage(rootCmd)
}
