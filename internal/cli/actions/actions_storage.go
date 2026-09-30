package actions

import (
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// StorageGet retrieves localStorage and/or sessionStorage items for the active tab.
func StorageGet(conn apiclient.Conn, cmd *cobra.Command, key string) {
	params := url.Values{}
	if t, _ := cmd.Flags().GetString("type"); t != "" {
		params.Set("type", t)
	}
	if key != "" {
		params.Set("key", key)
	}
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		params.Set("tabId", tab)
	}

	result := requireBytes(apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/storage", params), "Failed to get storage")
	buf := decodeMap(result, "Failed to parse response")
	printIndented(buf)
}

// StorageSet sets a single localStorage or sessionStorage item.
func StorageSet(conn apiclient.Conn, cmd *cobra.Command, key, value string) {
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

	requireMap(conn.Post("/storage", body), "Failed to set storage item")
}

// StorageDelete removes a single storage key.
func StorageDelete(conn apiclient.Conn, cmd *cobra.Command, key string) {
	if key == "" {
		usageErr(`Error: storage delete needs a key; to wipe the whole store use "pinchtab storage clear"`)
	}
	deleteStorage(conn, cmd, key)
}

// StorageClear clears storage (--type, or both stores with --all).
func StorageClear(conn apiclient.Conn, cmd *cobra.Command) {
	deleteStorage(conn, cmd, "")
}

func deleteStorage(conn apiclient.Conn, cmd *cobra.Command, key string) {
	storageType, _ := cmd.Flags().GetString("type")
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

	requireMap(apiclient.DoDelete(conn.Client, conn.Base, conn.Token, "/storage", nil, apiclient.WithBody(body)), "Failed to delete storage")
}
