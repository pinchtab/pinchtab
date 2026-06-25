package actions

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

func Wait(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	body := map[string]any{}

	textFlag, _ := cmd.Flags().GetString("text")
	notTextFlag, _ := cmd.Flags().GetString("not-text")
	urlFlag, _ := cmd.Flags().GetString("url")
	loadFlag, _ := cmd.Flags().GetString("load")
	fnFlag, _ := cmd.Flags().GetString("fn")
	stateFlag, _ := cmd.Flags().GetString("state")
	timeoutFlag, _ := cmd.Flags().GetInt("timeout")
	tabID, _ := cmd.Flags().GetString("tab")

	switch {
	case textFlag != "":
		body["text"] = textFlag
	case notTextFlag != "":
		body["notText"] = notTextFlag
	case urlFlag != "":
		body["url"] = urlFlag
	case loadFlag != "":
		body["load"] = loadFlag
	case fnFlag != "":
		body["fn"] = fnFlag
	case len(args) > 0:
		// Bare arg is overloaded: a number means a ms wait, anything else a selector.
		if ms, err := strconv.Atoi(args[0]); err == nil {
			body["ms"] = ms
		} else {
			body["selector"] = args[0]
			if stateFlag != "" {
				body["state"] = stateFlag
			}
		}
	default:
		fmt.Println("Usage: pinchtab wait <selector|ms> [--text|--not-text|--url|--load|--fn] [--timeout ms] [--tab id]")
		return
	}

	if timeoutFlag > 0 {
		body["timeout"] = timeoutFlag
	}

	path := "/wait"
	if tabID != "" {
		path = "/tabs/" + tabID + "/wait"
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, body)
		return
	}

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	// Server returns waited=false when condition isn't met within timeout
	if waited, ok := result["waited"].(bool); ok && !waited {
		output.Error("wait", "timeout", output.ExitTimeout)
		return
	}
	output.Success()
}
