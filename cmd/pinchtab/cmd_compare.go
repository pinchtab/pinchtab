package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var compareCmd = &cobra.Command{
	Use:   "compare <live-url> <staging-url>",
	Short: "Compare two site versions visually and by audit data",
	Long: `Audit the same pages on a live and a staging base URL, then compare
each pair: pixel-level visual diff on the screenshots plus data drift
(console errors, broken assets, accessibility score, load time).

Pages default to the two base URLs; pass --pages with comma-separated
relative paths to compare more. With --output-dir the comparison report is
written to <dir>/report.json and annotated diff images to <dir>/diffs/.

Differences are data, not failure: the command exits 0 even when pages
differ. Pass --fail-on-diff to exit non-zero when any visual or data diff
exists (CI gate).`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Compare(rt.client, rt.base, rt.token, cmd, args[0], args[1])
		})
	},
}
