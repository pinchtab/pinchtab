package main

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()

		if yolo, _ := cmd.Flags().GetBool("yolo"); yolo {
			if _, _, _, err := workflow.ApplyGuardsDownPreset(); err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("--yolo: %v", err)))
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, "YOLO mode: guards down"))
		}

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
	serverCmd.Flags().StringArrayP("extension", "e", nil, "Load browser extension (repeatable)")
	serverCmd.Flags().BoolP("headed", "H", false, "Start default instance in headed mode")
	serverCmd.Flags().BoolP("yolo", "y", false, "Apply guards down preset (enables evaluate, macro, download)")
	rootCmd.AddCommand(serverCmd)
}
