package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func Frame(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	if len(args) == 0 {
		params := url.Values{}
		if tabID != "" {
			params.Set("tabId", tabID)
		}
		if jsonOutput {
			apiclient.DoGet(client, base, token, "/frame", params)
			return
		}

		body := apiclient.DoGetRaw(client, base, token, "/frame", params)
		var resp struct {
			Current any  `json:"current"`
			Scoped  bool `json:"scoped"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Println(string(body))
			return
		}
		if !resp.Scoped {
			fmt.Println("main")
			return
		}
		if scope, ok := resp.Current.(map[string]any); ok {
			frameID, _ := scope["frameId"].(string)
			name, _ := scope["name"].(string)
			if name != "" {
				fmt.Printf("%s (%s)\n", frameID, name)
			} else {
				fmt.Println(frameID)
			}
		} else if s, ok := resp.Current.(string); ok {
			fmt.Println(s)
		}
		return
	}

	body := map[string]any{"target": args[0]}
	if tabID != "" {
		body["tabId"] = tabID
	}
	if jsonOutput {
		apiclient.DoPost(client, base, token, "/frame", body)
		return
	}

	result := apiclient.DoPostQuiet(client, base, token, "/frame", body)
	if scoped, ok := result["scoped"].(bool); ok && !scoped {
		fmt.Println("main")
		return
	}
	if scope, ok := result["current"].(map[string]any); ok {
		frameID, _ := scope["frameId"].(string)
		name, _ := scope["name"].(string)
		if name != "" {
			fmt.Printf("%s (%s)\n", frameID, name)
		} else {
			fmt.Println(frameID)
		}
	}
}
