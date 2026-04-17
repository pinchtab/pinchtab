package actions

import (
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func Frame(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	if len(args) == 0 {
		params := url.Values{}
		if tabID != "" {
			params.Set("tabId", tabID)
		}
		apiclient.DoGet(client, base, token, "/frame", params)
		return
	}

	body := map[string]any{"target": args[0]}
	if tabID != "" {
		body["tabId"] = tabID
	}
	apiclient.DoPost(client, base, token, "/frame", body)
}
