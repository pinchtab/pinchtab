package actions

import (
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// StorageGet retrieves localStorage and/or sessionStorage items for the active tab.
func StorageGet(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	if t, _ := cmd.Flags().GetString("type"); t != "" {
		params.Set("type", t)
	}
	if k, _ := cmd.Flags().GetString("key"); k != "" {
		params.Set("key", k)
	}
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		params.Set("tabId", tab)
	}

	result := requireBytes(apiclient.DoGetRaw(client, base, token, "/storage", params), 1, "Failed to get storage")
	buf := decodeMap(result, 1, "Failed to parse response")
	printIndented(buf)
}

// StorageSet sets a single localStorage or sessionStorage item.
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

// StorageDelete removes a storage item, clears a store, or clears both (--all).
// It calls DELETE /storage so the server-side delete/clear handler is used.
func StorageDelete(client *http.Client, base, token string, cmd *cobra.Command) {
	storageType, _ := cmd.Flags().GetString("type")
	key, _ := cmd.Flags().GetString("key")
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

// StorageClear clears storage (alias: passes type=all or the given type).
func StorageClear(client *http.Client, base, token string, cmd *cobra.Command) {
	StorageDelete(client, base, token, cmd)
}
