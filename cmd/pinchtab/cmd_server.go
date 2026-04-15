package main

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/report"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()
		cfg := loadConfig()
		applyInteractiveServerStartupChoices(cmd, cfg)
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

func applyInteractiveServerStartupChoices(cmd *cobra.Command, cfg *config.RuntimeConfig) {
	if cfg == nil || !isInteractiveTerminal() {
		return
	}

	startupPrompts, _ := cmd.Flags().GetBool("startup-prompts")
	if !startupPrompts {
		return
	}

	promptServerSecurityGuard(cfg)
	if !cmd.Flags().Changed("headed") {
		promptServerDefaultProfileMode(cfg)
	}
}

func promptServerSecurityGuard(cfg *config.RuntimeConfig) {
	posture := report.AssessSecurityPosture(cfg)

	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Security Guard"))
	fmt.Printf("  Current posture: %s %s\n", posture.Level, posture.Bar)

	picked, err := promptSelect("Choose guard for this run", []menuOption{
		{label: "Guard UP (locked / safer)", value: "up"},
		{label: "Guard DOWN (development)", value: "down"},
	})
	if err != nil {
		fmt.Println(cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("Invalid selection: %v (keeping current guard)", err)))
		return
	}

	switch picked {
	case "up":
		applyRuntimeGuardUp(cfg)
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "Guard UP selected for this run"))
	case "down":
		applyRuntimeGuardDown(cfg)
		fmt.Println(cli.StyleStdout(cli.WarningStyle, "Guard DOWN selected for this run"))
	default:
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "Keeping current guard settings"))
	}
}

func promptServerDefaultProfileMode(cfg *config.RuntimeConfig) {
	current := "headless"
	if !cfg.Headless {
		current = "headed"
	}

	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Default Profile Startup Mode"))
	fmt.Printf("  Current mode: %s\n", current)

	picked, err := promptSelect("Choose mode for default profile", []menuOption{
		{label: "Headed (visible browser)", value: "headed"},
		{label: "Headless (best for Docker/VPS)", value: "headless"},
	})
	if err != nil {
		fmt.Println(cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("Invalid selection: %v (keeping current mode)", err)))
		return
	}

	switch picked {
	case "headed":
		cfg.Headless = false
		cfg.HeadlessSet = true
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "Default profile mode set to headed for this run"))
	case "headless":
		cfg.Headless = true
		cfg.HeadlessSet = true
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "Default profile mode set to headless for this run"))
	default:
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "Keeping current default profile mode"))
	}
}

func applyRuntimeGuardUp(cfg *config.RuntimeConfig) {
	cfg.Bind = "127.0.0.1"
	cfg.AllowEvaluate = false
	cfg.AllowDownload = false
	cfg.AllowUpload = false
	cfg.AllowMacro = false
	cfg.AllowScreencast = false
	cfg.AllowedDomains = []string{"127.0.0.1", "localhost", "::1"}
	cfg.IDPI.Enabled = true
	cfg.IDPI.StrictMode = true
	cfg.IDPI.ScanContent = true
	cfg.IDPI.WrapContent = true
}

func applyRuntimeGuardDown(cfg *config.RuntimeConfig) {
	cfg.AllowEvaluate = true
	cfg.AllowDownload = true
	cfg.AllowUpload = true
	cfg.AllowMacro = true
	cfg.AllowScreencast = true
	cfg.AllowedDomains = nil
	cfg.IDPI.Enabled = false
	cfg.IDPI.StrictMode = false
	cfg.IDPI.ScanContent = false
	cfg.IDPI.WrapContent = false
}

func init() {
	serverCmd.GroupID = "primary"
	serverCmd.Flags().StringArray("extension", nil, "Load browser extension (repeatable)")
	serverCmd.Flags().Bool("headed", false, "Start default instance in headed mode")
	serverCmd.Flags().Bool("startup-prompts", true, "Prompt for security guard and default profile mode on interactive startup")
	rootCmd.AddCommand(serverCmd)
}
