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
			return browseractions.Audit(rt, cmd, target)
		})
	},
}

func configureAuditFlags() {
	auditCmd.Flags().Bool("sitemap", false, "Treat the URL as a sitemap.xml and audit the discovered pages")
	auditCmd.Flags().Int("sample-size", 0, "Pages audited per template group, e.g. /products/p1..pN (0 = all pages; deterministic picks)")
	auditCmd.Flags().Bool("screenshot", true, "Capture page screenshots")
	auditCmd.Flags().Bool("network-monitor", true, "Collect network requests and broken assets")
	auditCmd.Flags().String("output-dir", "", "Write report.json and screenshots/ to this directory")
	auditCmd.Flags().Int("concurrency", 0, "Pages audited in parallel (default 2, max 8)")
	auditCmd.Flags().Bool("json", false, "Print the full report JSON to stdout")
	auditCmd.Flags().String("seaportal-report", "", "Audit pages from a SeaPortal results JSON file (array of Result objects)")
	auditCmd.Flags().Bool("enrich-all", false, "Browser-enrich every seaportal page, ignoring browserRecommended routing")
	auditCmd.Flags().String("format", "json", "Report format: json, md, html, or pdf (pdf needs --output-dir and the evaluate capability; on print failure report.json is still written and the exit code is non-zero)")
	auditCmd.Flags().StringArray("cookie", nil, "Inject a cookie as name=value before the run (repeatable; the cookie jar is cleared afterwards)")
	auditCmd.Flags().String("cookies-file", "", "Inject cookies from a JSON array of {name, value, domain, ...} objects")
	auditCmd.Flags().String("profile", "", "Run against the instance of this browser profile")
}
