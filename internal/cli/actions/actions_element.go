package actions

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/spf13/cobra"
	"net/http"
	"strconv"
	"strings"
)

func Action(client *http.Client, base, token, kind, selectorArg string, cmd *cobra.Command) {
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
		// Explicit --css flag: send as plain CSS selector
		body["selector"] = css
	} else if selectorArg != "" {
		// Unified selector: parse and split into ref vs selector for the API
		setSelectorBody(body, selectorArg)
	} else if !hasXY {
		cli.Fatal("Usage: pinchtab %s <selector> or pinchtab %s --css <selector> or pinchtab %s --x <num> --y <num>", kind, kind, kind)
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

// setSelectorBody parses a unified selector string and sets the appropriate
// body fields. Ref selectors use the "ref" field; all others use "selector"
// with the raw value (no kind prefix — the server handles re-parsing).
func setSelectorBody(body map[string]any, s string) {
	sel := selector.Parse(s)
	switch sel.Kind {
	case selector.KindRef:
		body["ref"] = sel.Value
	default:
		body["selector"] = sel.Value
	}
}

func ActionSimple(client *http.Client, base, token, kind string, args []string, cmd *cobra.Command) {
	body := map[string]any{"kind": kind}

	switch kind {
	case "type":
		// First arg is a unified selector
		setSelectorBody(body, args[0])
		body["text"] = strings.Join(args[1:], " ")
	case "fill":
		// First arg is a unified selector
		setSelectorBody(body, args[0])
		body["text"] = strings.Join(args[1:], " ")
	case "press":
		body["key"] = args[0]
	case "scroll":
		sel := selector.Parse(args[0])
		if sel.Kind == selector.KindRef {
			body["ref"] = sel.Value
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
				cli.Fatal("Usage: pinchtab scroll <selector|pixels|direction>  (e.g. e5, 800, or down)")
			}
		}
	case "select":
		setSelectorBody(body, args[0])
		body["value"] = args[1]
	}

	tabID, _ := cmd.Flags().GetString("tab")
	path := "/action"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/action", tabID)
	}
	apiclient.DoPost(client, base, token, path, body)
}
