package actions

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

func Evaluate(conn apiclient.Conn, args []string, cmd *cobra.Command) {
	body := map[string]any{"expression": strings.Join(args, " ")}
	if awaitPromise, _ := cmd.Flags().GetBool("await-promise"); awaitPromise {
		body["awaitPromise"] = true
	}
	tabID, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tabID, "evaluate")

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Post(path, body)
		return
	}

	// Terse: scalar verbatim, object/array as compact JSON
	result := conn.PostQuiet(path, body)
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
	// A Promise returned without --await-promise prints {} on stdout; the hint on
	// stderr keeps stdout parseable while telling the caller why it was empty.
	if hint, ok := result["hint"].(string); ok && hint != "" {
		output.Hint(hint)
	}
}
