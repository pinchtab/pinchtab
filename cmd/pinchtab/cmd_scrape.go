package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var scrapeCmd = &cobra.Command{
	Use:   "scrape <url>",
	Short: "Scrape a site: HTTP crawl first, browser-render only pages that need it",
	Long: `Scrape a whole site into a page tree with markdown content. Pages are
discovered and extracted over plain HTTP first (via SeaPortal: sitemap or
link crawl, URL-pattern sampling). Pages whose HTTP extraction came back
thin, blocked, or failed are then re-rendered in the real browser (whatever
provider the instance runs — chrome, cloak, ghost-chrome) and re-extracted
from the rendered HTML, so JS-only content still lands in the report.

Each page records its content source (http or browser) and the routing
verdict. Pass --enrich-all to browser-render every page, or --no-browser
for the HTTP crawl alone. With --output-dir the report is written to
<dir>/report.json (and report.md with --format md); --json prints the full
report to stdout.

For a large site, survey it first with --preview: an outline of every page
(title, size, snippet, routing verdict) with the full bodies withheld and no
browser rendering, so it stays cheap to read. Then drill into the pages you
care about at full fidelity with --only <url> (repeatable), which scrapes
exactly those URLs instead of crawling.

Pages that fail in both engines do not fail the run: the command still
exits 0 and the failing page's report entry carries an "error" field.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCLIWithError(func(rt cliRuntime) error {
			return browseractions.Scrape(rt.client, rt.base, rt.token, cmd, args[0])
		})
	},
}

func configureScrapeFlags() {
	scrapeCmd.Flags().Int("max-pages", 0, "Maximum pages sampled across the site (default 50)")
	scrapeCmd.Flags().Int("max-per-pattern", 0, "Maximum pages sampled per URL pattern group (default 8)")
	scrapeCmd.Flags().StringArray("include", nil, "Only crawl URLs matching this regex (repeatable)")
	scrapeCmd.Flags().StringArray("exclude", nil, "Skip URLs matching this regex (repeatable)")
	scrapeCmd.Flags().Int("concurrency", 0, "Pages browser-rendered in parallel (default 2, max 8)")
	scrapeCmd.Flags().Bool("enrich-all", false, "Browser-render every reachable page, ignoring routing")
	scrapeCmd.Flags().Bool("no-browser", false, "HTTP crawl only; record routing verdicts without browser rendering")
	scrapeCmd.Flags().Bool("preview", false, "Outline only: page tree, sizes, and snippets, no browser rendering or full bodies — survey a large site before expanding")
	scrapeCmd.Flags().StringArray("only", nil, "Expand exactly these URLs at full fidelity instead of crawling (repeatable; drill down after --preview)")
	scrapeCmd.Flags().Int("timeout", 0, "Overall HTTP crawl timeout in seconds (default 60)")
	scrapeCmd.Flags().Bool("json", false, "Print the full report JSON to stdout")
	scrapeCmd.Flags().String("format", "json", "Report format: json or md")
	scrapeCmd.Flags().String("output-dir", "", "Write report.json (and report.md with --format md) to this directory")
	scrapeCmd.Flags().StringArray("cookie", nil, "Inject a cookie as name=value before the run (repeatable; the cookie jar is cleared afterwards)")
	scrapeCmd.Flags().String("cookies-file", "", "Inject cookies from a JSON array of {name, value, domain, ...} objects")
	scrapeCmd.Flags().String("profile", "", "Run against the instance of this browser profile")
}
