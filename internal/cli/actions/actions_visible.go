package actions

import (
	"encoding/json"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Visible checks whether an element identified by ref is visible on the page.
func Visible(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		output.Error("visible", "ref argument is required", 1)
		return
	}
	ref := args[0]

	params := url.Values{}
	params.Set("ref", ref)
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Get("/visible", params)
		return
	}

	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/visible", params)
	var result struct {
		Visible bool `json:"visible"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	if result.Visible {
		output.Value("true")
	} else {
		output.Value("false")
	}
}
