package actions

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// noElementsMatched is the one wording for the empty result, echoing the query so an agent
// can see what was actually searched. Both empty states in this command render it: the
// default path prints it and exits 0, the way `console` and `errors` state an empty result,
// while --ref-only prints it to stderr and exits non-zero.
//
// The two contracts are deliberate, not a leftover. --ref-only exists so a caller can write
// REF=$(pinchtab find … --ref-only) and spend the result on a click, and an empty REF is
// worse than a failed command. Unifying them in either direction would break one of the two
// callers, so what is shared is the sentence, not the exit code.
func noElementsMatched(query string) string {
	return fmt.Sprintf("No elements matched %q", query)
}

func Find(conn apiclient.Conn, query string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	threshold, _ := cmd.Flags().GetString("threshold")
	explain, _ := cmd.Flags().GetBool("explain")
	refOnly, _ := cmd.Flags().GetBool("ref-only")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	body := map[string]any{"query": query}
	if threshold != "" {
		body["threshold"] = threshold
	}
	if explain {
		body["explain"] = true
	}

	path := apiclient.TabPath(tabID, "find")
	capture := apiclient.CaptureVocab(tabID == "")

	if refOnly {
		result := conn.PostQuiet(path, body, capture)
		if tied := stringList(result["tied_refs"]); len(tied) > 0 {
			cli.Fatal("Ambiguous match for %q: %s are indistinguishable; pick one with %q (i counts from 1 in page order)", query, strings.Join(tied, ", "), result["retry_query"])
		}
		if ref, ok := result["best_ref"].(string); ok && ref != "" {
			fmt.Println(ref)
			return
		}
		cli.Fatal("%s", noElementsMatched(query))
	}

	if jsonOutput {
		conn.Post(path, body, capture)
		return
	}

	// Terse: one line per match: <ref>\t<role>\t"<name>"
	result := conn.PostQuiet(path, body, capture)
	matches, ok := result["matches"].([]any)
	if !ok || len(matches) == 0 {
		// Single result format
		if ref, ok := result["best_ref"].(string); ok && ref != "" {
			role, _ := result["role"].(string)
			name, _ := result["name"].(string)
			output.Value(fmt.Sprintf("%s\t%s\t%q", ref, role, name))
			return
		}
		output.Value(noElementsMatched(query))
		return
	}
	for _, m := range matches {
		if match, ok := m.(map[string]any); ok {
			ref, _ := match["ref"].(string)
			role, _ := match["role"].(string)
			name, _ := match["name"].(string)
			fmt.Printf("%s\t%s\t%q\n", ref, role, name)
		}
	}
}
