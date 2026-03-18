package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
)

// runSecurityWizard runs the interactive security setup wizard.
// isNew indicates a fresh install (full wizard) vs upgrade (migration notice).
// Returns true if the user completed setup, false if they cancelled.
func runSecurityWizard(cfg *config.FileConfig, configPath string, isNew bool) bool {
	interactive := isInteractiveTerminal()
	if _, err := config.EnsureFileToken(cfg); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("failed to generate auth token: %v", err)))
		return false
	}

	if !interactive {
		return runNonInteractiveSetup(cfg, configPath, isNew)
	}

	if isNew {
		return runFullWizard(cfg, configPath)
	}
	return runUpgradeNotice(cfg, configPath)
}

// runNonInteractiveSetup prints a summary and applies defaults silently.
func runNonInteractiveSetup(cfg *config.FileConfig, configPath string, isNew bool) bool {
	if isNew {
		fmt.Println()
		fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Know your config"))
		fmt.Println()
		fmt.Println("   Guard: UP (maximum security)")
		fmt.Printf("   Allowed domains: %s\n", strings.Join(getAllowedDomains(cfg), ", "))
		if cfg.Server.Token != "" {
			fmt.Printf("   Token: %s\n", cfg.Server.Token[:min(12, len(cfg.Server.Token))]+"...")
		}
		fmt.Println()
		fmt.Println("   Run " + cli.StyleStdout(cli.CommandStyle, "pinchtab security") + " to review all settings.")
		fmt.Println()
	} else {
		fmt.Println()
		fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Config updated to v"+config.CurrentConfigVersion))
		fmt.Println("   Run " + cli.StyleStdout(cli.CommandStyle, "pinchtab security") + " to review changes.")
		fmt.Println()
	}

	cfg.ConfigVersion = config.CurrentConfigVersion
	_ = config.SaveFileConfig(cfg, configPath)
	return true
}

// runFullWizard runs the interactive first-run wizard.
func runFullWizard(cfg *config.FileConfig, configPath string) bool {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Know your config"))
	fmt.Println()
	fmt.Println("PinchTab ships with the strongest security defaults.")
	fmt.Println("Choose your security posture:")
	fmt.Println()
	printSeparator()

	// Guard Up
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "1. Guard UP (recommended)"))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Only sites running on this machine can be automated."))
	fmt.Println()
	printSetting("domains", cli.StyleStdout(cli.ValueStyle, strings.Join(getAllowedDomains(cfg), ", ")))
	printSetting("evaluate", cli.StyleStdout(cli.SuccessStyle, "disabled"))
	printSetting("download", cli.StyleStdout(cli.SuccessStyle, "disabled"))
	printSetting("upload", cli.StyleStdout(cli.SuccessStyle, "disabled"))
	printSetting("macros", cli.StyleStdout(cli.SuccessStyle, "disabled"))
	printSetting("screencast", cli.StyleStdout(cli.SuccessStyle, "disabled"))
	printSetting("IDPI", cli.StyleStdout(cli.SuccessStyle, "strict"))
	fmt.Println()

	// Guard Down
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "2. Guard DOWN (development)"))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "All features enabled, any site can be automated. Use for local dev only."))
	fmt.Println()
	printSetting("domains", cli.StyleStdout(cli.WarningStyle, "all"))
	printSetting("evaluate", cli.StyleStdout(cli.WarningStyle, "enabled"))
	printSetting("download", cli.StyleStdout(cli.WarningStyle, "enabled"))
	printSetting("upload", cli.StyleStdout(cli.WarningStyle, "enabled"))
	printSetting("macros", cli.StyleStdout(cli.WarningStyle, "enabled"))
	printSetting("screencast", cli.StyleStdout(cli.WarningStyle, "enabled"))
	printSetting("IDPI", cli.StyleStdout(cli.WarningStyle, "off"))
	fmt.Println()
	printSeparator()
	fmt.Println()

	picked, err := promptSelect("Security posture", []menuOption{
		{label: "Guard UP — maximum security", value: "up"},
		{label: "Guard DOWN — development mode", value: "down"},
	})
	if err != nil {
		return false
	}

	switch picked {
	case "up":
		applyGuardUp(cfg)
	case "down":
		applyGuardDown(cfg)
	}

	// Dashboard access
	printSeparator()
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Dashboard"))
	fmt.Println()
	loginURL := dashboardURL(cfg, "")
	fmt.Println(cli.StyleStdout(cli.CommandStyle, loginURL))
	if err := copyToClipboard(loginURL); err == nil {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "Copied to clipboard"))
	}
	fmt.Println()

	// Save
	cfg.ConfigVersion = config.CurrentConfigVersion
	if err := config.SaveFileConfig(cfg, configPath); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("failed to save config: %v", err)))
		return false
	}

	fmt.Println(cli.StyleStdout(cli.SuccessStyle, "✓ Configuration complete — installing..."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "For more configuration, visit the dashboard."))
	fmt.Println()
	return true
}

// runUpgradeNotice shows a brief notice for config upgrades.
func runUpgradeNotice(cfg *config.FileConfig, configPath string) bool {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Config update (v"+config.CurrentConfigVersion+")"))
	fmt.Println()

	oldVersion := cfg.ConfigVersion
	if oldVersion == "" {
		oldVersion = "pre-0.8.0"
	}
	fmt.Printf("   Upgraded: %s → %s\n", oldVersion, config.CurrentConfigVersion)

	if cfg.Server.Token != "" {
		fmt.Printf("   Token: %s\n", cli.StyleStdout(cli.ValueStyle, cfg.Server.Token[:min(12, len(cfg.Server.Token))]+"..."))
	}

	fmt.Println()
	fmt.Println("   Run " + cli.StyleStdout(cli.CommandStyle, "pinchtab security") + " to review all settings.")
	fmt.Println()

	cfg.ConfigVersion = config.CurrentConfigVersion
	_ = config.SaveFileConfig(cfg, configPath)
	return true
}

// ─── Guard Presets ───────────────────────────────────────────────

func applyGuardUp(cfg *config.FileConfig) {
	f := false
	cfg.Security.AllowEvaluate = &f
	cfg.Security.AllowDownload = &f
	cfg.Security.AllowUpload = &f
	cfg.Security.AllowMacro = &f
	cfg.Security.AllowScreencast = &f
	cfg.Security.IDPI.Enabled = true
	cfg.Security.IDPI.StrictMode = true
	cfg.Security.IDPI.ScanContent = true
	cfg.Security.IDPI.WrapContent = true
	cfg.Security.IDPI.AllowedDomains = []string{"127.0.0.1", "localhost", "::1"}
	cfg.Server.Bind = "127.0.0.1"
}

func applyGuardDown(cfg *config.FileConfig) {
	t := true
	cfg.Security.AllowEvaluate = &t
	cfg.Security.AllowDownload = &t
	cfg.Security.AllowUpload = &t
	cfg.Security.AllowMacro = &t
	cfg.Security.AllowScreencast = &t
	cfg.Security.IDPI.Enabled = false
	cfg.Security.IDPI.StrictMode = false
	cfg.Security.IDPI.AllowedDomains = nil
}

// ─── Helpers ─────────────────────────────────────────────────────

func getAllowedDomains(cfg *config.FileConfig) []string {
	if len(cfg.Security.IDPI.AllowedDomains) > 0 {
		return cfg.Security.IDPI.AllowedDomains
	}
	return []string{"127.0.0.1", "localhost", "::1"}
}

func printSeparator() {
	fmt.Println(cli.StyleStdout(cli.MutedStyle, strings.Repeat("━", 50)))
}

func printSetting(name, value string) {
	fmt.Printf("  %-12s %s\n", name+":", value)
}

func dashboardURL(cfg *config.FileConfig, path string) string {
	host := orDefault(cfg.Server.Bind, "127.0.0.1")
	port := orDefault(cfg.Server.Port, "9867")
	return fmt.Sprintf("http://%s:%s%s", host, port, path)
}

func orDefault(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// copyToClipboard is defined in cmd_config.go
