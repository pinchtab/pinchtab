package actions

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func requireArgs(args []string, n int, usage string) {
	if len(args) < n {
		exitErr(2, "ERROR: "+usage)
	}
}

func postEmulation(client *http.Client, base, token string, cmd *cobra.Command, name string, body map[string]any, human func(map[string]any) string) {
	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/"+name)
	result := requireMap(apiclient.DoPostQuiet(client, base, token, path, body), 2, fmt.Sprintf("ERROR: set %s failed", name))

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		printIndented(result)
		return
	}
	fmt.Println(human(result))
}

func responseStatus(result map[string]any) string {
	status, _ := result["status"].(string)
	return status
}
