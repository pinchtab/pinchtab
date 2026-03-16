package actions

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// Activity lists recorded activity events.
func Activity(client *http.Client, base, token string, cmd *cobra.Command) {
	apiclient.DoGet(client, base, token, "/api/activity", activityParams(cmd, ""))
}

// ActivityTab lists recorded activity events for a specific tab.
func ActivityTab(client *http.Client, base, token, tabID string, cmd *cobra.Command) {
	apiclient.DoGet(client, base, token, "/api/activity", activityParams(cmd, tabID))
}

func activityParams(cmd *cobra.Command, tabID string) url.Values {
	params := url.Values{}
	if tabID != "" {
		params.Set("tabId", tabID)
	}
	if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if ageSec, _ := cmd.Flags().GetInt("age-sec"); ageSec > 0 {
		params.Set("ageSec", strconv.Itoa(ageSec))
	}
	return params
}
