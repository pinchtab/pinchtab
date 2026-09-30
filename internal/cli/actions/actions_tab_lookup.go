package actions

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

func resolveTabReference(conn apiclient.Conn, ref string) (string, error) {
	index, err := strconv.Atoi(ref)
	if err != nil {
		return ref, nil
	}
	if index < 1 {
		return "", fmt.Errorf("tab index %d out of range", index)
	}

	tabIDs, err := listTabIDs(conn)
	if err != nil {
		return "", err
	}
	if index > len(tabIDs) {
		return "", fmt.Errorf("tab index %d out of range (1-%d)", index, len(tabIDs))
	}
	return tabIDs[index-1], nil
}

func listTabIDs(conn apiclient.Conn) ([]string, error) {
	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/tabs", nil)
	var resp struct {
		Tabs []struct {
			ID string `json:"id"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Tabs != nil {
		ids := make([]string, 0, len(resp.Tabs))
		for _, tab := range resp.Tabs {
			if tab.ID != "" {
				ids = append(ids, tab.ID)
			}
		}
		return ids, nil
	}

	var tabs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &tabs); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		if tab.ID != "" {
			ids = append(ids, tab.ID)
		}
	}
	return ids, nil
}
