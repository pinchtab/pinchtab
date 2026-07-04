package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit <url>",
	Short: "Run a browser-level site audit",
	Long: `Audit one or more pages with full browser enrichment: screenshots,
console logs, network requests and broken assets, interactive elements,
accessibility score, and timing metrics.

The argument is a page URL, or a sitemap URL with --sitemap (pages are then
discovered from it). With --output-dir the report is written to
<dir>/report.json and screenshots to <dir>/screenshots/; --json prints the
full report to stdout.

Pages that fail to load do not fail the run: the command still exits 0 and
the failing page's report entry carries an "error" field.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Audit(rt.client, rt.base, rt.token, cmd, args[0])
		})
	},
}
