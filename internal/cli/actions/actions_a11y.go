package actions

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// A11yAudit runs GET /a11y/audit with the engine and filters its flags select,
// printing the full JSON envelope with --json or a short summary otherwise. The
// summary shape follows the engine: axe reports rule ids with impact and refs,
// native reports its scored findings.
func A11yAudit(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	engine, _ := cmd.Flags().GetString("engine")
	if axe, _ := cmd.Flags().GetBool("axe"); axe {
		engine = "axe"
	}
	if engine != "" {
		params.Set("engine", engine)
	}
	if v, _ := cmd.Flags().GetString("tags"); v != "" {
		params.Set("tags", v)
	}
	if v, _ := cmd.Flags().GetString("rules"); v != "" {
		params.Set("rules", v)
	}
	if inc, _ := cmd.Flags().GetBool("include-incomplete"); inc {
		params.Set("includeIncomplete", "true")
	}
	if v, _ := cmd.Flags().GetString("selector"); v != "" {
		params.Set("selector", v)
	}

	// The axe engine re-epochs the tab's ref cache and publishes a fresh
	// vocabulary token; capture it the way snapshot does (keyed by the resolved
	// tab, implicit when no --tab) so a later action on a returned ref echoes the
	// token the audit minted instead of a stale one the server would refuse 409.
	capture := apiclient.CaptureVocab(namedNoTab(cmd))
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		conn.Get("/a11y/audit", params, capture)
		return
	}
	printA11ySummary(apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/a11y/audit", params, capture))
}

// printA11ySummary renders a compact human summary for either engine, falling
// back to the raw body when it does not decode.
func printA11ySummary(body []byte) {
	var report struct {
		Engine     string `json:"engine"`
		Version    string `json:"version"`
		Score      int    `json:"score"`
		Violations []struct {
			ID     string `json:"id"`
			Impact string `json:"impact"`
			Nodes  []struct {
				Ref string `json:"ref"`
			} `json:"nodes"`
		} `json:"violations"`
		Findings []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
			Count    int    `json:"count"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		fmt.Println(string(body))
		return
	}

	if report.Engine == "axe" {
		fmt.Printf("engine=axe version=%s score=%d\n", report.Version, report.Score)
		for _, v := range report.Violations {
			refs := 0
			for _, n := range v.Nodes {
				if n.Ref != "" {
					refs++
				}
			}
			fmt.Printf("  %-24s %-8s nodes=%d refs=%d\n", v.ID, v.Impact, len(v.Nodes), refs)
		}
		return
	}

	fmt.Printf("engine=native score=%d\n", report.Score)
	for _, f := range report.Findings {
		fmt.Printf("  %-24s %-8s count=%d\n", f.Rule, f.Severity, f.Count)
	}
}
