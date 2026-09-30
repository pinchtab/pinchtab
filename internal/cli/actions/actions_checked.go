package actions

import (
	"encoding/json"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Checked checks whether an element identified by ref is checked.
func Checked(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		output.Error("checked", "ref argument is required", 1)
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
		conn.Get("/checked", params)
		return
	}

	body := conn.GetRaw("/checked", params)
	var result struct {
		Checked bool `json:"checked"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	if result.Checked {
		output.Value("true")
	} else {
		output.Value("false")
	}
}
