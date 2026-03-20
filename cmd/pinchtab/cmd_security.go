package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Review runtime security posture",
	Long:  "Shows runtime security posture and offers to restore recommended security defaults.",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadLocalConfig()
		handleSecurityCommand(cfg)
	},
}

func init() {
	securityCmd.GroupID = "config"
	securityCmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "Apply recommended security defaults",
		Run: func(cmd *cobra.Command, args []string) {
			handleSecurityUpCommand()
		},
	})
	securityCmd.AddCommand(&cobra.Command{
		Use:   "down",
		Short: "Lower guards while keeping loopback bind and API auth enabled",
		Run: func(cmd *cobra.Command, args []string) {
			handleSecurityDownCommand()
		},
	})
	rootCmd.AddCommand(securityCmd)
}

func handleSecurityCommand(cfg *config.RuntimeConfig) {
	interactive := isInteractiveTerminal()

	for {
		posture := cli.AssessSecurityPosture(cfg)
		warnings := cli.AssessSecurityWarnings(cfg)
		recommended := cli.RecommendedSecurityDefaultLines(cfg)

		printSecuritySummary(posture, interactive)

		if len(warnings) > 0 {
			fmt.Println()
			fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Warnings"))
			fmt.Println()
			for _, warning := range warnings {
				fmt.Printf("  - %s\n", cli.StyleStdout(cli.WarningStyle, warning.Message))
				for i := 0; i+1 < len(warning.Attrs); i += 2 {
					key, ok := warning.Attrs[i].(string)
					if !ok || key == "hint" {
						continue
					}
					fmt.Printf("      %s: %s\n", cli.StyleStdout(cli.MutedStyle, key), cli.StyleStdout(cli.ValueStyle, formatSecurityValue(warning.Attrs[i+1])))
				}
				for i := 0; i+1 < len(warning.Attrs); i += 2 {
					key, ok := warning.Attrs[i].(string)
					if ok && key == "hint" {
						fmt.Printf("      %s: %s\n", cli.StyleStdout(cli.MutedStyle, "hint"), cli.StyleStdout(cli.ValueStyle, formatSecurityValue(warning.Attrs[i+1])))
					}
				}
			}
		}

		if len(recommended) == 0 && len(warnings) == 0 {
			fmt.Println()
			fmt.Println("  " + cli.StyleStdout(cli.SuccessStyle, "All recommended security defaults are active."))
		} else if len(recommended) > 0 {
			fmt.Println()
			fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Recommended defaults"))
			fmt.Println()
			printRecommendedSecurityDefaults(recommended)
		}

		if !interactive {
			if len(recommended) > 0 {
				fmt.Println()
				fmt.Println(cli.StyleStdout(cli.MutedStyle, "Interactive editing skipped because stdin/stdout is not a terminal."))
			}
			return
		}

		nextCfg, changed, done, err := promptSecurityEdit(cfg, posture, len(recommended) > 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
			os.Exit(1)
		}
		if done {
			return
		}
		if !changed {
			fmt.Println()
			fmt.Println(cli.StyleStdout(cli.MutedStyle, "No changes made."))
			return
		}
		cfg = nextCfg
		fmt.Println()
	}
}

func formatSecurityValue(value any) string {
	switch v := value.(type) {
	case []string:
		return strings.Join(v, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func printRecommendedSecurityDefaults(lines []string) {
	for _, line := range lines {
		fmt.Printf("  - %s\n", cli.StyleStdout(cli.ValueStyle, line))
	}
}

func printSecuritySummary(posture cli.SecurityPosture, interactive bool) {
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Security"))
	fmt.Println()
	fmt.Printf("  %s  %s\n", posture.Bar, cli.StyleStdout(cli.ValueStyle, posture.Level))
	for i, check := range posture.Checks {
		indicator := cli.StyleStdout(cli.WarningStyle, "!!")
		if check.Passed {
			indicator = cli.StyleStdout(cli.SuccessStyle, "ok")
		}
		if interactive {
			fmt.Printf("  %d. %s %-20s %s\n", i+1, indicator, check.Label, check.Detail)
			continue
		}
		fmt.Printf("    %s %-20s %s\n", indicator, check.Label, check.Detail)
	}
}

func promptSecurityEdit(cfg *config.RuntimeConfig, posture cli.SecurityPosture, canRestoreDefaults bool) (*config.RuntimeConfig, bool, bool, error) {
	fmt.Println()
	prompt := "Edit item (1-8"
	if canRestoreDefaults {
		prompt += ", u = security up"
	}
	prompt += ", d = security down, blank to exit):"

	choice, err := promptInput(cli.StyleStdout(cli.HeadingStyle, prompt), "")
	if err != nil {
		return nil, false, false, err
	}
	choice = strings.ToLower(strings.TrimSpace(choice))
	if choice == "" {
		return nil, false, true, nil
	}

	if (choice == "u" || choice == "up") && canRestoreDefaults {
		nextCfg, changed, err := applySecurityUp()
		return nextCfg, changed, false, err
	}

	if choice == "d" || choice == "down" {
		nextCfg, changed, err := applySecurityDown()
		return nextCfg, changed, false, err
	}

	index := strings.TrimSpace(choice)
	for i, check := range posture.Checks {
		if index == fmt.Sprint(i+1) {
			nextCfg, changed, err := editSecurityCheck(cfg, check)
			return nextCfg, changed, false, err
		}
	}

	return nil, false, false, fmt.Errorf("invalid selection %q", choice)
}

func editSecurityCheck(cfg *config.RuntimeConfig, check cli.SecurityPostureCheck) (*config.RuntimeConfig, bool, error) {
	switch check.ID {
	case "bind_loopback":
		value, err := promptInput("Set server.bind:", cfg.Bind)
		if err != nil {
			return nil, false, err
		}
		return workflow.UpdateValue("server.bind", value)
	case "api_auth_enabled":
		picked, err := promptSelect("API authentication", []menuOption{
			{label: "Generate new token (Recommended)", value: "generate"},
			{label: "Set custom token", value: "custom"},
			{label: "Disable token", value: "disable"},
			{label: "Cancel", value: "cancel"},
		})
		if err != nil || picked == "" || picked == "cancel" {
			return cfg, false, nil
		}
		switch picked {
		case "generate":
			token, err := config.GenerateAuthToken()
			if err != nil {
				return nil, false, err
			}
			return workflow.UpdateValue("server.token", token)
		case "custom":
			token, err := promptInputHiddenDefault("Set server.token:", cfg.Token)
			if err != nil {
				return nil, false, err
			}
			return workflow.UpdateValue("server.token", token)
		case "disable":
			return workflow.UpdateValue("server.token", "")
		}
	case "sensitive_endpoints_disabled":
		current := strings.Join(cfg.EnabledSensitiveEndpoints(), ",")
		value, err := promptInput("Enable sensitive endpoints (evaluate,macro,screencast,download,upload; blank = disable all):", current)
		if err != nil {
			return nil, false, err
		}
		return workflow.UpdateSensitiveEndpoints(value)
	case "attach_disabled":
		picked, err := promptSelect("Attach endpoint", []menuOption{
			{label: "Disable (Recommended)", value: "disable"},
			{label: "Enable", value: "enable"},
			{label: "Cancel", value: "cancel"},
		})
		if err != nil || picked == "" || picked == "cancel" {
			return cfg, false, nil
		}
		return workflow.UpdateValue("security.attach.enabled", fmt.Sprintf("%t", picked == "enable"))
	case "attach_local_only":
		value, err := promptInput("Set security.attach.allowHosts (comma-separated):", strings.Join(cfg.AttachAllowHosts, ","))
		if err != nil {
			return nil, false, err
		}
		return workflow.UpdateValue("security.attach.allowHosts", value)
	case "idpi_whitelist_scoped":
		value, err := promptInput("Set security.idpi.allowedDomains (comma-separated):", strings.Join(cfg.IDPI.AllowedDomains, ","))
		if err != nil {
			return nil, false, err
		}
		return workflow.UpdateValue("security.idpi.allowedDomains", value)
	case "idpi_strict_mode":
		picked, err := promptSelect("IDPI strict mode", []menuOption{
			{label: "Enforce (Recommended)", value: "true"},
			{label: "Warn only", value: "false"},
			{label: "Cancel", value: "cancel"},
		})
		if err != nil || picked == "" || picked == "cancel" {
			return cfg, false, nil
		}
		return workflow.UpdateValue("security.idpi.strictMode", picked)
	case "idpi_content_protection":
		picked, err := promptSelect("IDPI content guard", []menuOption{
			{label: "Active: scan + wrap (Recommended)", value: "both"},
			{label: "Scan only", value: "scan"},
			{label: "Wrap only", value: "wrap"},
			{label: "Disable", value: "off"},
			{label: "Cancel", value: "cancel"},
		})
		if err != nil || picked == "" || picked == "cancel" {
			return cfg, false, nil
		}
		return workflow.UpdateContentGuard(picked)
	}
	return cfg, false, nil
}

func applySecurityUp() (*config.RuntimeConfig, bool, error) {
	configPath, changed, err := workflow.RestoreSecurityDefaults()
	if err != nil {
		return nil, false, fmt.Errorf("restore defaults: %w", err)
	}
	if !changed {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("Security defaults already match %s", configPath)))
		return config.Load(), false, nil
	}
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Security defaults restored in %s", configPath)))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Restart PinchTab to apply file-based changes."))
	return config.Load(), true, nil
}

func applySecurityDown() (*config.RuntimeConfig, bool, error) {
	nextCfg, configPath, changed, err := workflow.ApplyGuardsDownPreset()
	if err != nil {
		return nil, false, fmt.Errorf("guards down: %w", err)
	}
	if !changed {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("Guards down preset already matches %s", configPath)))
		return nextCfg, false, nil
	}
	fmt.Println(cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("Guards down preset applied in %s", configPath)))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Loopback bind and API auth remain enabled; sensitive endpoints and attach are enabled, IDPI is disabled."))
	return nextCfg, true, nil
}

func handleSecurityUpCommand() {
	if _, _, err := applySecurityUp(); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
}

func handleSecurityDownCommand() {
	if _, _, err := applySecurityDown(); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
}
