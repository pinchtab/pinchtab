package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func Evaluate(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	body := map[string]any{"expression": strings.Join(args, " ")}
	if awaitPromise, _ := cmd.Flags().GetBool("await-promise"); awaitPromise {
		body["awaitPromise"] = true
	}
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/evaluate"
	if tabID != "" {
		path = "/tabs/" + tabID + "/evaluate"
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, body)
		return
	}

	// Terse: scalar verbatim, object/array as compact JSON
	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if val, ok := result["result"]; ok {
		switch v := val.(type) {
		case string:
			fmt.Println(v)
		case float64, bool:
			fmt.Println(v)
		case nil:
			fmt.Println("null")
		default:
			if data, err := json.Marshal(v); err == nil {
				fmt.Println(string(data))
			} else {
				fmt.Println(v)
			}
		}
	}
}
