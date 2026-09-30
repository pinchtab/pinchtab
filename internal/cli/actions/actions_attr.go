package actions

import (
	"encoding/json"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Attr retrieves the value of an HTML attribute on an element identified by ref.
func Attr(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		output.Error("attr", "ref and name arguments are required", 1)
		return
	}
	ref := args[0]
	name := args[1]

	params := url.Values{}
	params.Set("ref", ref)
	params.Set("name", name)
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Get("/attr", params)
		return
	}

	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/attr", params)
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
