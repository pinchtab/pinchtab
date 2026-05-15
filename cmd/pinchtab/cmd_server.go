package main

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()

		cfg := loadConfig()
		backgroundMarker, _ := cmd.Flags().GetString("background-child")
		cfg.BackgroundMarker = backgroundMarker

		yolo, _ := cmd.Flags().GetBool("yolo")
		if yolo {
			fc, _, err := config.LoadFileConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("--yolo: load config: %v", err)))
				os.Exit(1)
			}
			if _, err := workflow.BuildGuardsDownConfig(fc); err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("--yolo: %v", err)))
				os.Exit(1)
			}
			config.ApplyFileConfigToRuntime(cfg, fc)
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, "YOLO mode: guards down for this run only (config file unchanged)"))
		}

		headed, _ := cmd.Flags().GetBool("headed")
		if headed {
			cfg.Headless = false
			cfg.HeadlessSet = true
		}
		exts, _ := cmd.Flags().GetStringArray("extension")
		if len(exts) > 0 {
			cfg.ExtensionPaths = append(cfg.ExtensionPaths, exts...)
		}
		verbose, _ := cmd.Flags().GetBool("verbose")
		cfg.VerboseStartup = verbose

		if background, _ := cmd.Flags().GetBool("background"); background {
			if err := runServerBackground(cfg, serverBackgroundOptions{
				Yolo:       yolo,
				Headed:     headed,
				Verbose:    verbose,
				Extensions: append([]string(nil), exts...),
			}); err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
				os.Exit(1)
			}
			return
		}
		server.RunDashboard(cfg, version)
	},
}

func init() {
	serverCmd.GroupID = "primary"
	serverCmd.Flags().StringArrayP("extension", "e", nil, "Load browser extension (repeatable)")
	serverCmd.Flags().BoolP("headed", "H", false, "Start default instance in headed mode")
	serverCmd.Flags().BoolP("yolo", "y", false, "Apply guards down preset (enables evaluate, macro, download, cookies)")
	serverCmd.Flags().BoolP("verbose", "v", false, "Show full startup banner and logs")
	serverCmd.Flags().BoolP("background", "b", false, "Spawn the server detached and return JSON with pid/url/token")
	serverCmd.Flags().String("background-child", "", "Internal marker for background server ownership")
	_ = serverCmd.Flags().MarkHidden("background-child")
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverRestartCmd)
	rootCmd.AddCommand(serverCmd)
}
