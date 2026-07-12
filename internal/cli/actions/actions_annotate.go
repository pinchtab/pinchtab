package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// Annotate injects (or clears) the persistent, clickable annotation overlay.
// On inject it prints a legend correlating each visible ref label with its role
// and accessible name, mirroring `screenshot --annotate`.
func Annotate(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("selector"); v != "" {
		params.Set("selector", v)
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	if clear, _ := cmd.Flags().GetBool("clear"); clear {
		params.Set("clear", "true")
		if apiclient.DoGetRaw(client, base, token, "/annotate", params) == nil {
			return
		}
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "Overlay cleared"))
		return
	}

	raw := apiclient.DoGetRaw(client, base, token, "/annotate", params)
	if raw == nil {
		return
	}
	var envelope struct {
		Annotated   int `json:"annotated"`
		Annotations []struct {
			Ref  string `json:"ref"`
			Role string `json:"role"`
			Name string `json:"name"`
			Tag  string `json:"tag"`
		} `json:"annotations"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		cli.Fatal("Decode failed: %v", err)
	}
	fmt.Println(cli.StyleStdout(cli.SuccessStyle,
		fmt.Sprintf("Annotated %d elements — click a label in the browser to copy its reference", envelope.Annotated)))
	for _, a := range envelope.Annotations {
		line := a.Ref
		if a.Role != "" {
			line += " " + a.Role
		}
		if a.Name != "" {
			line += " \"" + a.Name + "\""
		} else if a.Tag != "" {
			line += " <" + a.Tag + ">"
		}
		fmt.Println(line)
	}
}
