package actions

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// SetHeaders sets extra HTTP headers via the HTTP API.
func SetHeaders(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	requireArgs(args, 1, "set headers requires a JSON object argument")
	var headers map[string]string
	if err := json.Unmarshal([]byte(args[0]), &headers); err != nil {
		exitErr(2, fmt.Sprintf("ERROR: invalid JSON: %v", err))
	}

	postEmulation(client, base, token, cmd, "headers", map[string]any{"headers": headers}, func(map[string]any) string {
		return "applied"
	})
}
