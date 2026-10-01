package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

var compareCmd = &cobra.Command{
	Use:   "compare <live-url> <staging-url>",
	Short: "Compare two site versions visually and by audit data",
	Long: `Audit the same pages on a live and a staging base URL, then compare
each pair: pixel-level visual diff on the screenshots plus data drift
(uncaught JS errors, console errors, broken assets, accessibility score,
load time).

Uncaught JS errors drift on identity, not just count: an exception present
on one side only, or swapped for a different one, is a difference. The same
exception on both sides is not — this is a comparison gate, not an absolute
health check.

Pages default to the two base URLs; pass --pages with comma-separated
relative paths to compare more. With --output-dir the comparison report is
written to <dir>/report.json and annotated diff images to <dir>/diffs/.

Differences are data, not failure: the command exits 0 even when pages
differ. Pass --fail-on-diff to exit non-zero when any visual or data diff
exists (CI gate).`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCLIWithError(func(rt apiclient.Conn) error {
			return browseractions.Compare(rt, cmd, args[0], args[1])
		})
	},
}

func configureCompareFlags() {
	compareCmd.Flags().String("pages", "", "Comma-separated relative paths to compare (default: the base URLs)")
	compareCmd.Flags().Bool("visual-diff", true, "Capture screenshots and compute visual diffs")
	compareCmd.Flags().String("output-dir", "", "Write report.json and diffs/ to this directory")
	compareCmd.Flags().Int("concurrency", 0, "Pages audited in parallel per side (default 2, max 8)")
	compareCmd.Flags().Bool("json", false, "Print the comparison report JSON to stdout")
	compareCmd.Flags().Bool("fail-on-diff", false, "Exit non-zero when any visual or data diff exists")
	compareCmd.Flags().String("format", "json", "Report format: json, md, or html")
	compareCmd.Flags().StringArray("cookie", nil, "Inject a cookie as name=value before the run (repeatable; the cookie jar is cleared afterwards)")
	compareCmd.Flags().String("cookies-file", "", "Inject cookies from a JSON array of {name, value, domain, ...} objects")
	compareCmd.Flags().String("profile", "", "Run against the instance of this browser profile")
}
