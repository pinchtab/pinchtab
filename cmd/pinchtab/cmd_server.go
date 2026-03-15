package main

import (
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()
		cfg := config.Load()
		if exts, _ := cmd.Flags().GetStringArray("extension"); len(exts) > 0 {
			cfg.ExtensionPaths = append(cfg.ExtensionPaths, exts...)
		}
		server.RunDashboard(cfg, version)
	},
}

func init() {
	serverCmd.GroupID = "primary"
	serverCmd.Flags().StringArray("extension", nil, "Load browser extension (repeatable)")
	rootCmd.AddCommand(serverCmd)
}
