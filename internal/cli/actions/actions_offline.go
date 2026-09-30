package actions

import (
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetOffline enables or disables network offline emulation via the HTTP API.
func SetOffline(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	requireArgs(args, 1, "set offline requires <true|false> argument")
	offline, err := strconv.ParseBool(args[0])
	if err != nil {
		usageErr("ERROR: invalid offline value %q: must be true or false", args[0])
	}

	postEmulation(conn, cmd, "offline", map[string]any{"offline": offline}, func(map[string]any) string {
		if offline {
			return "offline"
		}
		return "online"
	})
}
