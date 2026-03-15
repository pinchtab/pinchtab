package actions

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
	"net/http"
)

// Back navigates the current (or specified) tab back in history.
func Back(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/back"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/back", tabID)
	}
	apiclient.DoPost(client, base, token, path, nil)
}

// Forward navigates the current (or specified) tab forward in history.
func Forward(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/forward"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/forward", tabID)
	}
	apiclient.DoPost(client, base, token, path, nil)
}

// Reload reloads the current (or specified) tab.
func Reload(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/reload"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/reload", tabID)
	}
	apiclient.DoPost(client, base, token, path, nil)
}

func Navigate(client *http.Client, base, token string, url string, cmd *cobra.Command) {
	body := map[string]any{"url": url}
	if v, _ := cmd.Flags().GetBool("new-tab"); v {
		body["newTab"] = true
	}
	if v, _ := cmd.Flags().GetBool("block-images"); v {
		body["blockImages"] = true
	}
	if v, _ := cmd.Flags().GetBool("block-ads"); v {
		body["blockAds"] = true
	}
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/navigate"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/navigate", tabID)
	}
	result := apiclient.DoPost(client, base, token, path, body)
	apiclient.SuggestNextAction("navigate", result)
}
