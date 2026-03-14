package actions

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
	"net/http"
	"strconv"
	"strings"
)

func isElementRef(value string) bool {
	if len(value) < 2 || value[0] != 'e' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func Action(client *http.Client, base, token, kind, refArg string, cmd *cobra.Command) {
	body := map[string]any{"kind": kind}

	css, _ := cmd.Flags().GetString("css")
	hasX := cmd.Flags().Changed("x")
	hasY := cmd.Flags().Changed("y")
	x, _ := cmd.Flags().GetFloat64("x")
	y, _ := cmd.Flags().GetFloat64("y")
	hasXY := hasX || hasY
	if hasXY {
		body["x"] = x
		body["y"] = y
		body["hasXY"] = true
	}

	if css != "" {
		body["selector"] = css
	} else if refArg != "" {
		body["ref"] = refArg
	} else if !hasXY {
		cli.Fatal("Usage: pinchtab %s <ref> or pinchtab %s --css <selector> or pinchtab %s --x <num> --y <num>", kind, kind, kind)
	}

	if kind == "click" {
		if v, _ := cmd.Flags().GetBool("wait-nav"); v {
			body["waitNav"] = true
		}
	}

	tabID, _ := cmd.Flags().GetString("tab")
	path := "/action"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/action", tabID)
	}
	apiclient.DoPost(client, base, token, path, body)
}

func ActionSimple(client *http.Client, base, token, kind string, args []string, cmd *cobra.Command) {
	body := map[string]any{"kind": kind}

	switch kind {
	case "type":
		body["ref"] = args[0]
		body["text"] = strings.Join(args[1:], " ")
	case "fill":
		if isElementRef(args[0]) {
			body["ref"] = args[0]
		} else {
			body["selector"] = args[0]
		}
		body["text"] = strings.Join(args[1:], " ")
	case "press":
		body["key"] = args[0]
	case "scroll":
		if strings.HasPrefix(args[0], "e") {
			body["ref"] = args[0]
		} else if px, err := strconv.Atoi(args[0]); err == nil {
			body["scrollY"] = px
		} else {
			switch strings.ToLower(args[0]) {
			case "down":
				body["scrollY"] = 800
			case "up":
				body["scrollY"] = -800
			case "right":
				body["scrollX"] = 800
			case "left":
				body["scrollX"] = -800
			default:
				cli.Fatal("Usage: pinchtab scroll <ref|pixels|direction>  (e.g. e5, 800, or down)")
			}
		}
	case "select":
		body["ref"] = args[0]
		body["value"] = args[1]
	}

	tabID, _ := cmd.Flags().GetString("tab")
	path := "/action"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/action", tabID)
	}
	apiclient.DoPost(client, base, token, path, body)
}
