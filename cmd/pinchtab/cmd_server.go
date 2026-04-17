package main

import (
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()
		cfg := loadConfig()
		if headed, _ := cmd.Flags().GetBool("headed"); headed {
			cfg.Headless = false
			cfg.HeadlessSet = true
		}
		if exts, _ := cmd.Flags().GetStringArray("extension"); len(exts) > 0 {
			cfg.ExtensionPaths = append(cfg.ExtensionPaths, exts...)
		}
		server.RunDashboard(cfg, version)
	},
}

func init() {
	serverCmd.GroupID = "primary"
	serverCmd.Flags().StringArray("extension", nil, "Load browser extension (repeatable)")
	serverCmd.Flags().Bool("headed", false, "Start default instance in headed mode")
	rootCmd.AddCommand(serverCmd)
}
