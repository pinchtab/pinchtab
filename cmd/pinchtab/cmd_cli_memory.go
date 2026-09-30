package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "JavaScript heap usage, heap snapshots and their summaries",
	Long: `Read the tab's JavaScript heap usage and DOM counters.

Pass --gc to run a garbage collection first so two reads compare live memory only.
Subcommands: snapshot writes a V8 heap snapshot to a server-side file, summary
lists its top constructors and duplicate strings, and compare reports what grew
between two snapshots. All three need security.allowMemory,
because a heap snapshot holds every string on the page, tokens included.`,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Memory(rt.Client, rt.Base, rt.Token, cmd)
		})
	},
}

var memorySnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Take a V8 heap snapshot to a server-side file",
	Long: `Take a V8 heap snapshot of the tab. The server streams it to its own
heapsnapshots directory and prints the id, path, size and node count; pass the id
to 'pinchtab memory summary'. --out copies the file to a local path when the server
shares this machine's filesystem. The file loads in Chrome DevTools (Memory panel).`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.MemorySnapshot(rt.Client, rt.Base, rt.Token, cmd)
		})
	},
}

var memorySummaryCmd = &cobra.Command{
	Use:   "summary <id>",
	Short: "Summarize a saved heap snapshot",
	Long:  "Summarize a saved heap snapshot: top constructors by self size and by count, total node and edge counts, and the largest duplicate strings.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.MemorySummary(rt.Client, rt.Base, rt.Token, cmd, args[0])
		})
	},
}

var memoryCompareCmd = &cobra.Command{
	Use:   "compare <base> <head>",
	Short: "Compare two saved heap snapshots by constructor growth",
	Long: `Compare two saved heap snapshots: per constructor, count and self size in each and
the delta, largest change first, plus duplicate strings new in head. Take a snapshot,
run the suspect action, take another, then compare the two ids. --retained adds each
listed constructor's retained size in head from a dominator tree, which costs memory
and time proportional to the snapshot's edges.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.MemoryCompare(rt.Client, rt.Base, rt.Token, cmd, args[0], args[1])
		})
	},
}

func configureMemoryFlags() {
	memoryCmd.Flags().Bool("gc", false, "Collect garbage before reading")
	memoryCmd.Flags().String("tab", "", "Target tab ID")
	memoryCmd.Flags().Bool("json", false, "Output raw JSON")
	memorySnapshotCmd.Flags().String("tab", "", "Target tab ID")
	memorySnapshotCmd.Flags().String("out", "", "Copy the snapshot to this local path")
	memorySnapshotCmd.Flags().Bool("json", false, "Output raw JSON")
	memorySummaryCmd.Flags().Int("top", 20, "Rows per table (max 200)")
	memorySummaryCmd.Flags().Bool("json", false, "Output raw JSON")
	memoryCompareCmd.Flags().Int("top", 20, "Constructor and duplicate-string rows (max 200)")
	memoryCompareCmd.Flags().Bool("retained", false, "Add retained sizes from the head snapshot's dominator tree")
	memoryCompareCmd.Flags().Bool("json", false, "Output raw JSON")
}
