package actions

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// Wait sends a wait request to the server.
func Wait(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	body := map[string]any{}

	textFlag, _ := cmd.Flags().GetString("text")
	urlFlag, _ := cmd.Flags().GetString("url")
	loadFlag, _ := cmd.Flags().GetString("load")
	fnFlag, _ := cmd.Flags().GetString("fn")
	stateFlag, _ := cmd.Flags().GetString("state")
	timeoutFlag, _ := cmd.Flags().GetInt("timeout")
	tabID, _ := cmd.Flags().GetString("tab")

	switch {
	case textFlag != "":
		body["text"] = textFlag
	case urlFlag != "":
		body["url"] = urlFlag
	case loadFlag != "":
		body["load"] = loadFlag
	case fnFlag != "":
		body["fn"] = fnFlag
	case len(args) > 0:
		// Check if arg is a number (ms wait) or a selector
		if ms, err := strconv.Atoi(args[0]); err == nil {
			body["ms"] = ms
		} else {
			body["selector"] = args[0]
			if stateFlag != "" {
				body["state"] = stateFlag
			}
		}
	default:
		fmt.Println("Usage: pinchtab wait <selector|ms> [--text|--url|--load|--fn] [--timeout ms] [--tab id]")
		return
	}

	if timeoutFlag > 0 {
		body["timeout"] = timeoutFlag
	}

	path := "/wait"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/wait", tabID)
	}

	apiclient.DoPost(client, base, token, path, body)
}
