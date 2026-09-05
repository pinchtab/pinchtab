package actions

import (
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func StorageGet(client *http.Client, base, token string, cmd *cobra.Command, key string) {
	params := url.Values{}
	if t, _ := cmd.Flags().GetString("type"); t != "" {
		params.Set("type", t)
	}
	if k := storageKey(cmd, key); k != "" {
		params.Set("key", k)
	}
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		params.Set("tabId", tab)
	}

	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/storage", params), 1, "Failed to get storage")
	buf := decodeMap(result, 1, "Failed to parse response")
	printIndented(buf)
}

func StorageSet(client *http.Client, base, token string, cmd *cobra.Command, key, value string) {
	storageType, _ := cmd.Flags().GetString("type")
	if storageType == "" {
		storageType = "local"
	}
	tabID, _ := cmd.Flags().GetString("tab")

	body := map[string]any{
		"key":   key,
		"value": value,
		"type":  storageType,
	}
	if tabID != "" {
		body["tabId"] = tabID
	}

	requireMap(apiclient.DoPost(client, base, token, "/storage", body), 1, "Failed to set storage item")
}

func storageKey(cmd *cobra.Command, positional string) string {
	if positional != "" {
		return positional
	}
	key, _ := cmd.Flags().GetString("key")
	return key
}

func StorageDelete(client *http.Client, base, token string, cmd *cobra.Command, positional string) {
	storageType, _ := cmd.Flags().GetString("type")
	key := storageKey(cmd, positional)
	all, _ := cmd.Flags().GetBool("all")
	tabID, _ := cmd.Flags().GetString("tab")

	if all {
		storageType = "all"
	}
	if storageType == "" {
		storageType = "local"
	}

	body := map[string]any{
		"type": storageType,
	}
	if key != "" {
		body["key"] = key
	}
	if tabID != "" {
		body["tabId"] = tabID
	}

	requireMap(apiclient.DoDeleteJSON(client, base, token, "/storage", body), 1, "Failed to delete storage")
}

func StorageClear(client *http.Client, base, token string, cmd *cobra.Command) {
	StorageDelete(client, base, token, cmd, "")
}
