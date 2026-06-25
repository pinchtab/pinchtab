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

func runNonInteractiveSetup(cfg *config.FileConfig, configPath string, isNew bool) bool {
	if isNew {
		fmt.Println()
		fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Know your config"))
		fmt.Println()
		fmt.Println("   Guard: UP (maximum security)")
		fmt.Printf("   Allowed domains: %s\n", strings.Join(getAllowedDomains(cfg), ", "))
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

func runFullWizard(cfg *config.FileConfig, configPath string) bool {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Know your config"))
	fmt.Println()
	fmt.Println("PinchTab ships with the strongest security defaults.")
	fmt.Println("Choose your security posture:")
	fmt.Println()
	printSeparator()

	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "1. Guard UP (recommended)"))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Only sites running on this machine can be automated."))
	fmt.Println()
	printPosture(guardUpPosture)
	fmt.Println()

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "2. Guard DOWN (development)"))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "All features enabled, any site can be automated. Use for local dev only."))
	fmt.Println()
	printPosture(guardDownPosture)
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
		applyPosture(cfg, guardUpPosture)
	case "down":
		applyPosture(cfg, guardDownPosture)
	}

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

func runUpgradeNotice(cfg *config.FileConfig, configPath string) bool {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "🛡️  Config update (v"+config.CurrentConfigVersion+")"))
	fmt.Println()

	oldVersion := cfg.ConfigVersion
	if oldVersion == "" {
		oldVersion = "pre-0.8.0"
	}
	fmt.Printf("   Upgraded: %s → %s\n", oldVersion, config.CurrentConfigVersion)

	fmt.Println()
	fmt.Println("   Run " + cli.StyleStdout(cli.CommandStyle, "pinchtab security") + " to review all settings.")
	fmt.Println()

	cfg.ConfigVersion = config.CurrentConfigVersion
	_ = config.SaveFileConfig(cfg, configPath)
	return true
}

// securityPosture is the single source of truth for a wizard security choice:
// both the printed summary (printPosture) and the saved config (applyPosture) are
// derived from it, so they cannot drift.
type securityPosture struct {
	allowEvaluate   bool
	allowDownload   bool
	allowCookies    bool
	allowUpload     bool
	allowMacro      bool
	allowScreencast bool
	idpiOn          bool     // drives IDPI Enabled/StrictMode/ScanContent/WrapContent
	allowedDomains  []string // nil → "all"
	bind            string   // "" → leave Server.Bind unchanged
}

var guardUpPosture = securityPosture{
	idpiOn:         true,
	allowedDomains: []string{"127.0.0.1", "localhost", "::1"},
	bind:           "127.0.0.1",
}

var guardDownPosture = securityPosture{
	allowEvaluate:   true,
	allowDownload:   true,
	allowCookies:    true,
	allowUpload:     true,
	allowMacro:      true,
	allowScreencast: true,
	idpiOn:          false,
	allowedDomains:  nil,
	bind:            "",
}

func boolPtr(b bool) *bool { return &b }

func applyPosture(cfg *config.FileConfig, p securityPosture) {
	cfg.Security.AllowEvaluate = boolPtr(p.allowEvaluate)
	cfg.Security.AllowDownload = boolPtr(p.allowDownload)
	cfg.Security.AllowCookies = boolPtr(p.allowCookies)
	cfg.Security.AllowUpload = boolPtr(p.allowUpload)
	cfg.Security.AllowMacro = boolPtr(p.allowMacro)
	cfg.Security.AllowScreencast = boolPtr(p.allowScreencast)
	cfg.Security.IDPI.Enabled = p.idpiOn
	cfg.Security.IDPI.StrictMode = p.idpiOn
	cfg.Security.IDPI.ScanContent = p.idpiOn
	cfg.Security.IDPI.WrapContent = p.idpiOn
	cfg.Security.AllowedDomains = append([]string(nil), p.allowedDomains...)
	if p.bind != "" {
		cfg.Server.Bind = p.bind
	}
}

func printPosture(p securityPosture) {
	printSetting("domains", styledDomains(p.allowedDomains))
	printSetting("evaluate", styledToggle(p.allowEvaluate))
	printSetting("download", styledToggle(p.allowDownload))
	printSetting("upload", styledToggle(p.allowUpload))
	printSetting("macros", styledToggle(p.allowMacro))
	printSetting("screencast", styledToggle(p.allowScreencast))
	printSetting("IDPI", styledIDPI(p.idpiOn))
}

func styledToggle(allowed bool) string {
	if allowed {
		return cli.StyleStdout(cli.WarningStyle, "enabled")
	}
	return cli.StyleStdout(cli.SuccessStyle, "disabled")
}

func styledIDPI(on bool) string {
	if on {
		return cli.StyleStdout(cli.SuccessStyle, "strict")
	}
	return cli.StyleStdout(cli.WarningStyle, "off")
}

func styledDomains(domains []string) string {
	if len(domains) == 0 {
		return cli.StyleStdout(cli.WarningStyle, "all")
	}
	return cli.StyleStdout(cli.ValueStyle, strings.Join(domains, ", "))
}

func getAllowedDomains(cfg *config.FileConfig) []string {
	if len(cfg.Security.AllowedDomains) > 0 {
		return cfg.Security.AllowedDomains
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
