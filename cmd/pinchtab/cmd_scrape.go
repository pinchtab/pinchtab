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
