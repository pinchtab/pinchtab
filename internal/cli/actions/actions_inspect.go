package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/spf13/cobra"
)

func Title(client *http.Client, base, token string, cmd *cobra.Command) {
	inspectGet(client, base, token, "/title", cmd, nil, func(body []byte) {
		var result struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			output.Value(string(body))
			return
		}
		output.Value(result.Title)
	})
}

func URL(client *http.Client, base, token string, cmd *cobra.Command) {
	inspectGet(client, base, token, "/url", cmd, nil, func(body []byte) {
		var result struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			output.Value(string(body))
			return
		}
		output.Value(result.URL)
	})
}

func HTML(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	inspectGet(client, base, token, "/html", cmd, args, func(body []byte) {
		var result struct {
			HTML string `json:"html"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			output.Value(string(body))
			return
		}
		output.Value(result.HTML)
	})
}

func CSS(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	inspectGet(client, base, token, "/css", cmd, args, func(body []byte) {
		var result struct {
			CSS map[string]any `json:"css"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			output.Value(string(body))
			return
		}
		if prop, _ := cmd.Flags().GetString("prop"); prop != "" {
			if v, ok := result.CSS[prop]; ok {
				output.Value(fmt.Sprint(v))
				return
			}
			output.Value("")
			return
		}
		pretty, err := json.MarshalIndent(result.CSS, "", "  ")
		if err != nil {
			output.Value(fmt.Sprint(result.CSS))
			return
		}
		output.Value(string(pretty))
	})
}

func inspectGet(client *http.Client, base, token, path string, cmd *cobra.Command, args []string, terse func([]byte)) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	if v, _ := cmd.Flags().GetString("frame"); v != "" {
		params.Set("frameId", v)
	}
	if v, _ := cmd.Flags().GetString("max-chars"); v != "" {
		params.Set("maxChars", v)
	}
	if v, _ := cmd.Flags().GetString("prop"); v != "" {
		params.Set("prop", v)
	}

	selectorArg := ""
	if len(args) > 0 {
		selectorArg = args[0]
	} else if v, _ := cmd.Flags().GetString("selector"); v != "" {
		selectorArg = v
	}
	if selectorArg != "" {
		sel := selector.Parse(selectorArg)
		if sel.Kind == selector.KindRef {
			params.Set("ref", sel.Value)
		} else {
			params.Set("selector", selectorArg)
		}
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoGet(client, base, token, path, params)
		return
	}
	body := apiclient.DoGetRaw(client, base, token, path, params)
	terse(body)
}
