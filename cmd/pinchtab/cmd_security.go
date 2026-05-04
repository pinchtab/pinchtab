package main

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Review runtime security posture",
	Long:  "Shows runtime security posture and recommended defaults.",
	Run: func(cmd *cobra.Command, args []string) {
		printSecurityOverview(loadLocalConfig())
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
		Short: "Apply a documented security-reducing preset while keeping loopback bind and API auth enabled",
		Long: "Applies the guards-down preset for local operator workflows. " +
			"This is a documented, non-default, security-reducing configuration change: " +
			"sensitive endpoint families and attach are enabled, while IDPI protections are disabled. " +
			"Loopback bind and API authentication remain enabled, and attach host allowlisting stays local-only until you widen it explicitly.",
		Run: func(cmd *cobra.Command, args []string) {
			handleSecurityDownCommand()
		},
	})
	rootCmd.AddCommand(securityCmd)
}

func printSecurityOverview(cfg *config.RuntimeConfig) {
	posture := cli.AssessSecurityPosture(cfg)
	recommended := cli.RecommendedSecurityDefaultLines(cfg)
	warnings := cli.AssessSecurityWarnings(cfg)

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Security"))
	fmt.Println()
	for _, check := range posture.Checks {
		style := cli.ValueStyle
		if !check.Passed {
			style = cli.WarningStyle
		}
		fmt.Printf("  %-20s %s\n", check.Label, cli.StyleStdout(style, check.Detail))
	}
	fmt.Println()

	if len(recommended) == 0 && len(warnings) == 0 {
		fmt.Println("  " + cli.StyleStdout(cli.SuccessStyle, "All recommended security defaults are active."))
	} else {
		label := fmt.Sprintf("%d setting(s) differ from recommended defaults —", len(recommended))
		if len(recommended) == 0 {
			label = fmt.Sprintf("%d security warning(s) detected —", len(warnings))
		}
		fmt.Printf("  %s %s\n",
			cli.StyleStdout(cli.MutedStyle, label),
			cli.StyleStdout(cli.CommandStyle, "pinchtab security up"))
	}
	fmt.Println()

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Change security:"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab security up"), cli.StyleStdout(cli.MutedStyle, "# restore recommended defaults"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab security down"), cli.StyleStdout(cli.MutedStyle, "# apply guards-down preset (persistent)"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab server -y"), cli.StyleStdout(cli.MutedStyle, "# guards down for one run only (in-memory)"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config set <path> <value>"), cli.StyleStdout(cli.MutedStyle, "# tune individual security flags"))
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
	fmt.Println(cli.StyleStdout(cli.WarningStyle, "This is a documented, non-default, security-reducing preset."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Loopback bind and API auth remain enabled; sensitive endpoints and attach are enabled, and IDPI protections are disabled."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Attach host allowlisting remains local-only. Widening allowHosts or enabling bridge schemes later is an additional explicit weakening."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Changing server.bind away from 127.0.0.1 later is also an additional explicit weakening unless another network boundary still constrains access."))
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
