package actions

import (
	"encoding/json"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Value retrieves the .value property of a form element identified by ref.
func Value(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		output.Error("value", "ref argument is required", 1)
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
		conn.Get("/value", params)
		return
	}

	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/value", params)
	var result struct {
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	if result.Value == nil {
		output.Value("null")
	} else {
		output.Value(*result.Value)
	}
}
