package actions

import (
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetMedia emulates a CSS media feature via the HTTP API.
func SetMedia(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	requireArgs(args, 2, "set media requires <feature> <value> arguments")
	postEmulation(conn, cmd, "media", map[string]any{"feature": args[0], "value": args[1]}, responseStatus)
}
