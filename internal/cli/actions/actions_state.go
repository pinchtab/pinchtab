package actions

import (
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

const stateNameRequired = "Error: a state name is required, as <name> or --name"

func StateCurrent(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	params := url.Values{}
	if tabID != "" {
		params.Set("tabId", tabID)
	}

	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/state", params), "Failed to read current browser state")
	buf := decodeMap(result, "Failed to parse response")
	printIndented(buf)
}

func StateList(client *http.Client, base, token string) {
	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/state/list", nil), "Failed to list state files")
	buf := decodeMap(result, "Failed to parse response")
	printIndented(buf)
}

func StateSave(client *http.Client, base, token string, cmd *cobra.Command, name string) {
	encrypt, _ := cmd.Flags().GetBool("encrypt")
	tabID, _ := cmd.Flags().GetString("tab")

	body := map[string]any{
		"name":    name,
		"encrypt": encrypt,
	}
	if tabID != "" {
		body["tabId"] = tabID
	}

	requireMap(apiclient.DoPost(client, base, token, "/state/save", body), "Failed to save state")
}

// StateLoad supports exact name or prefix-based loading (most recent match).
func StateLoad(client *http.Client, base, token string, cmd *cobra.Command, name string) {
	tabID, _ := cmd.Flags().GetString("tab")

	if name == "" {
		usageErr("%s", stateNameRequired)
	}

	body := map[string]any{
		"name": name,
	}
	if tabID != "" {
		body["tabId"] = tabID
	}

	requireMap(apiclient.DoPost(client, base, token, "/state/load", body), "Failed to load state")
}

func StateShow(client *http.Client, base, token string, name string) {
	if name == "" {
		usageErr("%s", stateNameRequired)
	}

	params := url.Values{}
	params.Set("name", name)

	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/state/show", params), "Failed to show state")
	buf := decodeMap(result, "Failed to parse response")
	printIndented(buf)
}

func StateDelete(client *http.Client, base, token string, name string) {
	if name == "" {
		usageErr("%s", stateNameRequired)
	}

	params := url.Values{}
	params.Set("name", name)

	requireMap(apiclient.DoDelete(client, base, token, "/state", params), "Failed to delete state")
}

func StateClean(client *http.Client, base, token string, cmd *cobra.Command) {
	hours, _ := cmd.Flags().GetInt("older-than")

	body := map[string]any{
		"olderThanHours": hours,
	}

	requireMap(apiclient.DoPost(client, base, token, "/state/clean", body), "Failed to clean state files")
}
