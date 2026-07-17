package main

import (
	"fmt"

	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit [url]",
	Short: "Run a browser-level site audit",
	Long: `Audit one or more pages with full browser enrichment: screenshots,
console logs, network requests and broken assets, interactive elements,
accessibility score, and timing metrics.

The argument is a page URL, or a sitemap URL with --sitemap (pages are then
discovered from it). With --seaportal-report the pages come from a SeaPortal
results JSON file instead, and only pages SeaPortal marked browserRecommended
are browser-enriched (the rest keep their HTTP-extraction summary); pass
--enrich-all to browser-enrich every page. With --output-dir the report is
written to <dir>/report.json and screenshots to <dir>/screenshots/; --json
prints the full report to stdout.

Pages that fail to load do not fail the run: the command still exits 0 and
the failing page's report entry carries an "error" field.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		seaportalReport, _ := cmd.Flags().GetString("seaportal-report")
		if len(args) == 0 && seaportalReport == "" {
			return fmt.Errorf("audit needs a URL argument or --seaportal-report")
		}
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return runCLIWithError(func(rt cliRuntime) error {
			return browseractions.Audit(rt.client, rt.base, rt.token, cmd, target)
		})
	},
}
