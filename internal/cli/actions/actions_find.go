package actions

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

func Find(client *http.Client, base, token string, query string, cmd *cobra.Command) {
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

	path := "/find"
	if tabID != "" {
		path = "/tabs/" + tabID + "/find"
	}

	if refOnly {
		result := apiclient.DoPostQuiet(client, base, token, path, body)
		if ref, ok := result["best_ref"].(string); ok && ref != "" {
			fmt.Println(ref)
			return
		}
		cli.Fatal("No element found")
	}

	if jsonOutput {
		apiclient.DoPost(client, base, token, path, body)
		return
	}

	// Terse: one line per match: <ref>\t<role>\t"<name>"
	result := apiclient.DoPostQuiet(client, base, token, path, body)
	matches, ok := result["matches"].([]any)
	if !ok || len(matches) == 0 {
		// Single result format
		if ref, ok := result["best_ref"].(string); ok && ref != "" {
			role, _ := result["role"].(string)
			name, _ := result["name"].(string)
			output.Value(fmt.Sprintf("%s\t%s\t%q", ref, role, name))
		}
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
