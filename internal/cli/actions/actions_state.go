package actions

import (
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func StateCurrent(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	params := url.Values{}
	if tabID != "" {
		params.Set("tabId", tabID)
	}

	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/state", params), 1, "Failed to read current browser state")
	buf := decodeMap(result, 1, "Failed to parse response")
	printIndented(buf)
}

func StateList(client *http.Client, base, token string) {
	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/state/list", nil), 1, "Failed to list state files")
	buf := decodeMap(result, 1, "Failed to parse response")
	printIndented(buf)
}

func StateSave(client *http.Client, base, token string, cmd *cobra.Command) {
	name, _ := cmd.Flags().GetString("name")
	encrypt, _ := cmd.Flags().GetBool("encrypt")
	tabID, _ := cmd.Flags().GetString("tab")

	body := map[string]any{
		"name":    name,
		"encrypt": encrypt,
	}
	if tabID != "" {
		body["tabId"] = tabID
	}

	requireMap(apiclient.DoPost(client, base, token, "/state/save", body), 1, "Failed to save state")
}

// StateLoad supports exact name or prefix-based loading (most recent match).
func StateLoad(client *http.Client, base, token string, cmd *cobra.Command) {
	name, _ := cmd.Flags().GetString("name")
	tabID, _ := cmd.Flags().GetString("tab")

	if name == "" {
		exitErr(1, "Error: --name is required")
	}

	body := map[string]any{
		"name": name,
	}
	if tabID != "" {
		body["tabId"] = tabID
	}

	requireMap(apiclient.DoPost(client, base, token, "/state/load", body), 1, "Failed to load state")
}

func StateShow(client *http.Client, base, token string, cmd *cobra.Command) {
	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		exitErr(1, "Error: --name is required")
	}

	params := url.Values{}
	params.Set("name", name)

	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/state/show", params), 1, "Failed to show state")
	buf := decodeMap(result, 1, "Failed to parse response")
	printIndented(buf)
}

func StateDelete(client *http.Client, base, token string, cmd *cobra.Command) {
	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		exitErr(1, "Error: --name is required")
	}

	params := url.Values{}
	params.Set("name", name)

	requireMap(apiclient.DoDelete(client, base, token, "/state", params), 1, "Failed to delete state")
}

func StateClean(client *http.Client, base, token string, cmd *cobra.Command) {
	hours, _ := cmd.Flags().GetInt("older-than")

	body := map[string]any{
		"olderThanHours": hours,
	}

	requireMap(apiclient.DoPost(client, base, token, "/state/clean", body), 1, "Failed to clean state files")
}
