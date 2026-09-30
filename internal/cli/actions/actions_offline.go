package actions

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// SetOffline enables or disables network offline emulation via the HTTP API.
func SetOffline(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	requireArgs(args, 1, "set offline requires <true|false> argument")
	offline, err := strconv.ParseBool(args[0])
	if err != nil {
		exitErr(2, fmt.Sprintf("ERROR: invalid offline value %q: must be true or false", args[0]))
	}

	postEmulation(client, base, token, cmd, "offline", map[string]any{"offline": offline}, func(map[string]any) string {
		if offline {
			return "offline"
		}
		return "online"
	})
}
