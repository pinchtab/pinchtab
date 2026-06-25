package actions

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/spf13/cobra"
)

func Text(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	params := url.Values{}
	// --full is the preferred, discoverable name; --raw is kept as a
	// backward-compatible alias. Both switch the server off its default
	// Readability extraction onto a plain document.body.innerText pull, so
	// navigation / repeated headlines / short text nodes that Readability
	// considers chrome are retained.
	raw, _ := cmd.Flags().GetBool("raw")
	full, _ := cmd.Flags().GetBool("full")
	if raw || full {
		params.Set("mode", "raw")
		params.Set("format", "text")
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	// --frame lets a one-shot call target an iframe without having to go
	// through the stateful /frame scope. If omitted, the handler falls back
	// to the currently-scoped frame for the tab (set via `pinchtab frame`).
	if v, _ := cmd.Flags().GetString("frame"); v != "" {
		params.Set("frameId", v)
	}

	selectorStr := ""
	if len(args) > 0 {
		selectorStr = args[0]
	} else if v, _ := cmd.Flags().GetString("selector"); v != "" {
		selectorStr = v
	}
	if selectorStr != "" {
		sel := selector.Parse(selectorStr)
		if sel.Kind == selector.KindRef {
			params.Set("ref", sel.Value)
		} else {
			params.Set("selector", selectorStr)
		}
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoGet(client, base, token, "/text", params)
		return
	}

	body := apiclient.DoGetRaw(client, base, token, "/text", params)
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	output.Value(result.Text)
}
