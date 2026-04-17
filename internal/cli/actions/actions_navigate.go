package actions

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
	"net/http"
	"os"
)

// stdoutIsPipe reports whether stdout is a pipe/redirect rather than a
// terminal. Used to switch `nav` into machine-friendly output mode so
// `export PINCHTAB_TAB=$(pinchtab nav URL)` captures just the tab ID.
func stdoutIsPipe() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}

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

	// Machine-friendly output: emit only the tabId when --print-tab-id is set
	// or when stdout is a pipe/redirect. Enables:
	//   export PINCHTAB_TAB=$(pinchtab nav http://example.com)
	printTabID, _ := cmd.Flags().GetBool("print-tab-id")
	if printTabID || stdoutIsPipe() {
		result := apiclient.DoPostQuiet(client, base, token, path, body)
		if tid, ok := result["tabId"].(string); ok && tid != "" {
			fmt.Println(tid)
		}
		return
	}

	result := apiclient.DoPost(client, base, token, path, body)
	apiclient.SuggestNextAction("navigate", result)
}
