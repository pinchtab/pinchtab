package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

// a11yCmd is a bare group: the shared unknown-subcommand guard installs its
// argument validator and help action, so setting Args or RunE here would opt it
// out of the refusal that names its subcommands.
var a11yCmd = &cobra.Command{
	Use:   "a11y",
	Short: "Accessibility auditing",
	Long:  "Accessibility auditing. Subcommand: audit — score the page and list violations (native engine, or axe-core with --axe).",
}

var a11yAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit the current page for accessibility issues",
	Long: `Audit the current page for accessibility issues.

The default native engine scores the page over the accessibility snapshot. Pass
--axe to run vendored axe-core in the page instead: it returns industry rule ids
(image-alt, label, color-contrast, …) with WCAG tags, help URLs and, where a
failing element maps to a snapshot ref, that ref — so you can act on it with
'pinchtab action'. Filter an axe run with --tags (WCAG tag list) or --rules (a
rule-id allowlist).`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.A11yAudit(rt, cmd)
		})
	},
}

func configureA11yFlags() {
	a11yAuditCmd.Flags().Bool("axe", false, "Run the axe-core engine instead of the native scan (engine=axe)")
	a11yAuditCmd.Flags().String("engine", "", "Audit engine: native (default) or axe")
	a11yAuditCmd.Flags().String("tags", "", "axe: comma list of WCAG tags to run, e.g. wcag2a,wcag2aa,best-practice")
	a11yAuditCmd.Flags().String("rules", "", "axe: comma list of rule ids to run (allowlist)")
	a11yAuditCmd.Flags().Bool("include-incomplete", false, "axe: include rules axe could not decide (incomplete)")
	a11yAuditCmd.Flags().String("selector", "", "Scope the audit to a CSS selector")
	a11yAuditCmd.Flags().String("tab", "", "Target tab ID")
	a11yAuditCmd.Flags().Bool("json", false, "Output raw JSON")
}
