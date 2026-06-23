package actions

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// CacheClear clears the browser's HTTP disk cache.
func CacheClear(client *http.Client, base, token string, cmd *cobra.Command) {
	result := requireMap(apiclient.DoPostQuiet(client, base, token, "/cache/clear", nil), 2, "ERROR: cache: clear failed")

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		printIndented(result)
	} else {
		fmt.Println("OK")
	}
}

// CacheStatus checks if the browser cache can be cleared.
func CacheStatus(client *http.Client, base, token string, cmd *cobra.Command) {
	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/cache/status", nil), 2, "ERROR: cache: status check failed")
	buf := decodeMap(result, 2, "ERROR: cache")

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		printIndented(buf)
	} else {
		if canClear, ok := buf["canClear"].(bool); ok && canClear {
			fmt.Println("can-clear")
		} else {
			fmt.Println("cache-empty")
		}
	}
}
