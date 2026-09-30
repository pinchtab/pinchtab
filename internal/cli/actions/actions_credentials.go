package actions

import (
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetCredentials sets HTTP auth credentials via the HTTP API.
func SetCredentials(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	requireArgs(args, 2, "set credentials requires <username> <password> arguments")
	postEmulation(conn, cmd, "credentials", map[string]any{"username": args[0], "password": args[1]}, responseStatus)
}
