package actions

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
	"net/http"
	"strings"
)

func Evaluate(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	body := map[string]any{"expression": strings.Join(args, " ")}
	if awaitPromise, _ := cmd.Flags().GetBool("await-promise"); awaitPromise {
		body["awaitPromise"] = true
	}
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/evaluate"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/evaluate", tabID)
	}
	apiclient.DoPost(client, base, token, path, body)
}
