package actions

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func requireArgs(args []string, n int, usage string) {
	if len(args) < n {
		usageErr("ERROR: %s", usage)
	}
}

func postEmulation(conn apiclient.Conn, cmd *cobra.Command, name string, body map[string]any, human func(map[string]any) string) {
	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/"+name)
	result := requireMap(conn.PostQuiet(path, body), fmt.Sprintf("ERROR: set %s failed", name))

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
