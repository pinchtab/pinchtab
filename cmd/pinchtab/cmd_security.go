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
	Long:  "Shows runtime security posture and recommended defaults.",
	Run: func(cmd *cobra.Command, args []string) {
		printSecurityOverview(loadLocalConfig())
	},
}

var securityDryRun bool

func init() {
	securityCmd.GroupID = "config"
	up := &cobra.Command{
		Use:   "up",
		Short: "Apply recommended security defaults",
		Run: func(cmd *cobra.Command, args []string) {
			handleSecurityUpCommand()
		},
	}
	down := &cobra.Command{
		Use:   "down",
		Short: "Apply a documented security-reducing preset while keeping loopback bind and API auth enabled",
		Long: "Applies the guards-down preset for local operator workflows. " +
			"This is a documented, non-default, security-reducing configuration change: " +
			"sensitive endpoint families and attach are enabled, while IDPI protections are disabled. " +
			"Loopback bind and API authentication remain enabled, and attach host allowlisting stays local-only until you widen it explicitly.",
		Run: func(cmd *cobra.Command, args []string) {
			handleSecurityDownCommand()
		},
	}
	for _, preset := range []*cobra.Command{up, down} {
		preset.Flags().BoolVar(&securityDryRun, "dry-run", false, "Report the settings the preset would write without writing them")
		securityCmd.AddCommand(preset)
	}
	rootCmd.AddCommand(securityCmd)
}

// printEnforcedDriftWarning flags when a server is running but was started with
// a different config than what's now on disk — i.e. the posture printed below is
// the user's intent, not necessarily what the live server enforces. The IDPI
// guard and allowlist are snapshotted at boot, so an edit without a restart left
// the running server enforcing stale policy with nothing signalling it; this is
// that signal.
func printEnforcedDriftWarning(cfg *config.RuntimeConfig) {
	if cfg == nil {
		return
	}
	localToken, err := resolveCLIToken(cfg, resolveDefaultCLIBase(cfg))
	if err != nil {
		return
	}
	snap, state := fetchHealthSnapshotWithToken(cfg.Port, localToken)
	if state != healthSnapshotRunning || snap == nil || !snap.RestartRequired {
		return
	}
	fmt.Println(cli.StyleStdout(cli.WarningStyle,
		"  ⚠ The running server started with a different config — the settings below are not all in effect yet."))
	if len(snap.RestartReasons) > 0 {
		fmt.Printf("    Pending (needs restart): %s\n", strings.Join(snap.RestartReasons, ", "))
	}
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "    Apply with: pinchtab server restart"))
	fmt.Println()
}

func printSecurityOverview(cfg *config.RuntimeConfig) {
	printEnforcedDriftWarning(cfg)
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

	switch {
	case len(recommended) == 0 && len(warnings) == 0:
		fmt.Println("  " + cli.StyleStdout(cli.SuccessStyle, "All recommended security defaults are active."))
	case len(recommended) > 0:
		fmt.Printf("  %s %s\n",
			cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("%d config setting(s) differ from recommended defaults (the rows above summarise them) —", len(recommended))),
			cli.StyleStdout(cli.CommandStyle, "pinchtab security up"))
		for _, line := range recommended {
			fmt.Printf("    %s\n", cli.StyleStdout(cli.WarningStyle, line))
		}
	default:
		fmt.Println("  " + cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("%d security warning(s) detected:", len(warnings))))
		for _, warning := range warnings {
			fmt.Printf("    %s\n", cli.StyleStdout(cli.WarningStyle, warning.Message))
			if hint := warning.Hint(); hint != "" {
				fmt.Printf("      %s\n", cli.StyleStdout(cli.MutedStyle, hint))
			}
		}
	}
	fmt.Println()

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Change security:"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab security up"), cli.StyleStdout(cli.MutedStyle, "# restore recommended defaults"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab security down"), cli.StyleStdout(cli.MutedStyle, "# apply guards-down preset (persistent)"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab server -y"), cli.StyleStdout(cli.MutedStyle, "# guards down for one run only (in-memory)"))
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config set <path> <value>"), cli.StyleStdout(cli.MutedStyle, "# tune individual security flags"))
}

func applySecurityUp(dryRun bool) (workflow.PresetResult, error) {
	result, err := workflow.RestoreSecurityDefaults(dryRun)
	if err != nil {
		return result, fmt.Errorf("restore defaults: %w", err)
	}
	if !result.Changed() {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("Security defaults already match %s", result.ConfigPath)))
		return result, nil
	}
	if dryRun {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("Security defaults would write %d config setting(s) to %s:", len(result.Changes), result.ConfigPath)))
		printSettingChanges(result.Changes)
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "Nothing was written (--dry-run)."))
		return result, nil
	}
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Security defaults restored in %s (%d config setting(s) written):", result.ConfigPath, len(result.Changes))))
	printSettingChanges(result.Changes)
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Restart PinchTab to apply file-based changes."))
	return result, nil
}

func applySecurityDown(dryRun bool) (workflow.PresetResult, error) {
	result, err := workflow.ApplyGuardsDownPreset(dryRun)
	if err != nil {
		return result, fmt.Errorf("guards down: %w", err)
	}
	if !result.Changed() {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, fmt.Sprintf("Guards down preset already matches %s", result.ConfigPath)))
		return result, nil
	}
	if dryRun {
		fmt.Println(cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("Guards down preset would write %d config setting(s) to %s:", len(result.Changes), result.ConfigPath)))
		printSettingChanges(result.Changes)
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "Nothing was written (--dry-run)."))
		return result, nil
	}
	fmt.Println(cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("Guards down preset applied in %s (%d config setting(s) written):", result.ConfigPath, len(result.Changes))))
	printSettingChanges(result.Changes)
	fmt.Println(cli.StyleStdout(cli.WarningStyle, "This is a documented, non-default, security-reducing preset."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Loopback bind and API auth remain enabled; sensitive endpoints and attach are enabled, and IDPI protections are disabled."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Attach host allowlisting remains local-only. Widening allowHosts or enabling bridge schemes later is an additional explicit weakening."))
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Changing server.bind away from 127.0.0.1 later is also an additional explicit weakening unless another network boundary still constrains access."))
	return result, nil
}

// printSettingChanges lists each key in the shape the residual warnings use, and
// says which posture row shows it, so nothing a preset writes is invisible on
// both sides.
func printSettingChanges(changes []workflow.SettingChange) {
	for _, change := range changes {
		fmt.Printf("    %s\n", cli.StyleStdout(cli.WarningStyle, fmt.Sprintf("%s: %s -> %s", change.Path, settingValue(change.Old), settingValue(change.New))))
		fmt.Printf("      %s\n", cli.StyleStdout(cli.MutedStyle, postureRowNote(change.Path)))
	}
}

func settingValue(v string) string {
	if v == "" {
		return "<absent>"
	}
	return v
}

func postureRowNote(path string) string {
	rows := cli.PostureRowsForSetting(path)
	if len(rows) == 0 {
		return "not shown in the posture table"
	}
	return "posture row: " + strings.Join(rows, ", ")
}

func handleSecurityUpCommand() {
	if _, err := applySecurityUp(securityDryRun); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
}

func handleSecurityDownCommand() {
	if _, err := applySecurityDown(securityDryRun); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
}
