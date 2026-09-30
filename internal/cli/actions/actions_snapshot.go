package actions

import (
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func Snapshot(conn apiclient.Conn, cmd *cobra.Command, selectorOverride string) {
	params := url.Values{}
	full, _ := cmd.Flags().GetBool("full")
	if !full {
		if v, _ := cmd.Flags().GetBool("interactive"); v {
			params.Set("filter", "interactive")
		}
	}
	params.Set("format", snapshotFormat(cmd))
	if v, _ := cmd.Flags().GetBool("diff"); v {
		params.Set("diff", "true")
	}
	if v, _ := cmd.Flags().GetString("selector"); v != "" {
		params.Set("selector", v)
	} else if selectorOverride != "" {
		params.Set("selector", selectorOverride)
	}
	if v, _ := cmd.Flags().GetString("max-tokens"); v != "" {
		params.Set("maxTokens", v)
	}
	if v, _ := cmd.Flags().GetString("depth"); v != "" {
		params.Set("depth", v)
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	result := conn.Get("/snapshot", params, apiclient.CaptureVocab(namedNoTab(cmd)))
	apiclient.SuggestNextAction("snapshot", result)
}

func snapshotFormat(cmd *cobra.Command) string {
	if v, _ := cmd.Flags().GetBool("text"); v {
		return "text"
	}
	if v, _ := cmd.Flags().GetBool("json"); v {
		return "json"
	}
	if v, _ := cmd.Flags().GetBool("full"); v {
		return "json"
	}
	if v, _ := cmd.Flags().GetBool("compact"); v {
		return "compact"
	}
	return "json"
}
