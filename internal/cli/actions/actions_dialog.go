package actions

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Dialog handles a JavaScript dialog (accept or dismiss).
func Dialog(client *http.Client, base, token string, action string, text string, cmd *cobra.Command) {
	body := map[string]any{"action": action}
	if text != "" {
		body["text"] = text
	}
	tabID, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tabID, "dialog")

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, body)
		return
	}

	apiclient.DoPostQuiet(client, base, token, path, body)
	output.Success()
}
