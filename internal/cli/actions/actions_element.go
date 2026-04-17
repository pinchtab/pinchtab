package actions

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/spf13/cobra"
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
	}

	if button, _ := cmd.Flags().GetString("button"); button != "" {
		body["button"] = button
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
		// --dialog-action arms a one-shot JS dialog handler before the click.
		// Mirrors the HTTP action body field {"dialogAction":"accept"|"dismiss"}.
		// Without this, a click that opens an alert/confirm hangs until
		// /dialog is called from a separate request.
		if v, _ := cmd.Flags().GetString("dialog-action"); v != "" {
			body["dialogAction"] = v
		}
		if v, _ := cmd.Flags().GetString("dialog-text"); v != "" {
			body["dialogText"] = v
		}
	}

	postAction(client, base, token, cmd, body)
}

// setSelectorBody parses a unified selector string and sets the appropriate
// body fields. Ref selectors use the "ref" field; all other kinds use
// "selector" and MUST retain their kind prefix (e.g. `text:Submit`,
// `xpath://button`, `semantic:accept button`) so the server can re-parse
// them correctly. For auto-detected CSS (no prefix on input), the raw
// input is forwarded verbatim.
func setSelectorBody(body map[string]any, s string) {
	sel := selector.Parse(s)
	switch sel.Kind {
	case selector.KindRef:
		body["ref"] = sel.Value
	case selector.KindCSS:
		// CSS is the default kind — the original string either had a
		// `css:` prefix or was auto-detected. Send the value without a
		// prefix; the server will re-parse as CSS.
		body["selector"] = sel.Value
	default:
		// text:, xpath:, semantic:, find: — preserve the original input
		// so the server sees the correct kind prefix when it re-parses.
		body["selector"] = s
	}
}

func postAction(client *http.Client, base, token string, cmd *cobra.Command, body map[string]any) {
	postActionWithHeaders(client, base, token, cmd, body, nil)
}

func postActionWithHeaders(client *http.Client, base, token string, cmd *cobra.Command, body map[string]any, headers map[string]string) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/action"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/action", tabID)
	}
	apiclient.DoPostWithHeaders(client, base, token, path, body, headers)
}

func setPointBody(body map[string]any, x, y float64) {
	body["x"] = x
	body["y"] = y
}

func readWheelDelta(cmd *cobra.Command, primary string) (int, bool) {
	if cmd.Flags().Changed(primary) {
		if value, err := cmd.Flags().GetInt(primary); err == nil {
			return value, true
		}
	}
	return 0, false
}

func parseCoordinateArgs(xArg, yArg string) (float64, float64, error) {
	x, err := strconv.ParseFloat(xArg, 64)
	if err != nil {
		return 0, 0, err
	}
	y, err := strconv.ParseFloat(yArg, 64)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func applyMouseTarget(body map[string]any, selectorArg string, cmd *cobra.Command) bool {
	css, _ := cmd.Flags().GetString("css")
	hasX := cmd.Flags().Changed("x")
	hasY := cmd.Flags().Changed("y")
	if hasX || hasY {
		x, _ := cmd.Flags().GetFloat64("x")
		y, _ := cmd.Flags().GetFloat64("y")
		setPointBody(body, x, y)
		return true
	}
	if css != "" {
		body["selector"] = css
		return true
	}
	if selectorArg != "" {
		setSelectorBody(body, selectorArg)
		return true
	}
	return false
}

func MouseAction(client *http.Client, base, token, kind string, args []string, cmd *cobra.Command) {
	body := map[string]any{"kind": kind}

	if button, _ := cmd.Flags().GetString("button"); button != "" {
		body["button"] = button
	}

	switch kind {
	case bridge.ActionMouseMove:
		if len(args) == 2 {
			if cmd.Flags().Changed("x") || cmd.Flags().Changed("y") || cmd.Flags().Changed("css") {
				cli.Fatal("Usage: pinchtab mouse move <x> <y> or pinchtab mouse move <selector> or pinchtab mouse move --x <num> --y <num>")
			}
			x, y, err := parseCoordinateArgs(args[0], args[1])
			if err != nil {
				cli.Fatal("Usage: pinchtab mouse move <x> <y>")
			}
			setPointBody(body, x, y)
		} else if len(args) == 1 {
			if cmd.Flags().Changed("x") || cmd.Flags().Changed("y") || cmd.Flags().Changed("css") {
				cli.Fatal("Usage: pinchtab mouse move <x> <y> or pinchtab mouse move <selector> or pinchtab mouse move --x <num> --y <num>")
			}
			setSelectorBody(body, args[0])
		} else if !applyMouseTarget(body, "", cmd) {
			cli.Fatal("Usage: pinchtab mouse move <x> <y> or pinchtab mouse move <selector> or pinchtab mouse move --x <num> --y <num>")
		}
	case bridge.ActionMouseDown, bridge.ActionMouseUp:
		if len(args) > 1 {
			cli.Fatal("Usage: pinchtab mouse %s [selector]", strings.TrimPrefix(kind, "mouse-"))
		}
		_ = applyMouseTarget(body, optionalMouseArg(args), cmd)
	case bridge.ActionMouseWheel:
		if len(args) > 1 {
			cli.Fatal("Usage: pinchtab mouse wheel <dy> [--dx <n>] or pinchtab mouse wheel [selector]")
		}
		if len(args) == 1 {
			if dy, err := strconv.Atoi(args[0]); err == nil {
				body["deltaY"] = dy
			} else {
				setSelectorBody(body, args[0])
			}
		}
		if deltaX, ok := readWheelDelta(cmd, "dx"); ok {
			body["deltaX"] = deltaX
		}
		if deltaY, ok := readWheelDelta(cmd, "dy"); ok {
			if _, fromArg := body["deltaY"]; fromArg {
				cli.Fatal("Usage: pinchtab mouse wheel <dy> [--dx <n>] or pinchtab mouse wheel [selector]")
			}
			body["deltaY"] = deltaY
		}
		if _, hasTarget := body["selector"]; !hasTarget {
			if _, hasRef := body["ref"]; !hasRef {
				_ = applyMouseTarget(body, "", cmd)
			}
		} else if cmd.Flags().Changed("x") || cmd.Flags().Changed("y") || cmd.Flags().Changed("css") {
			cli.Fatal("Usage: pinchtab mouse wheel <dy> [--dx <n>] or pinchtab mouse wheel [selector]")
		}
	default:
		cli.Fatal("unsupported mouse action: %s", kind)
	}

	postAction(client, base, token, cmd, body)
}

type dragTarget struct {
	selector string
	x        float64
	y        float64
	hasXY    bool
}

func optionalMouseArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func parseDragTarget(raw string) dragTarget {
	parts := strings.Split(raw, ",")
	if len(parts) == 2 {
		x, errX := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		y, errY := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errX == nil && errY == nil {
			return dragTarget{x: x, y: y, hasXY: true}
		}
	}
	return dragTarget{selector: raw}
}

func actionBodyForTarget(kind string, target dragTarget) map[string]any {
	body := map[string]any{"kind": kind}
	if target.hasXY {
		setPointBody(body, target.x, target.y)
		return body
	}
	setSelectorBody(body, target.selector)
	return body
}

func Drag(client *http.Client, base, token string, args []string, cmd *cobra.Command) {
	// Two modes:
	//   1. pinchtab drag <selector> --drag-x N --drag-y N
	//        → single HTTP "drag" action with pixel offsets (dragX/dragY).
	//   2. pinchtab drag <from> <to>
	//        → synthesized mouse-move → mouse-down → mouse-move → mouse-up
	//          sequence. Each target may be "selector" or "x,y" coords.
	hasDragX := cmd.Flags().Changed("drag-x")
	hasDragY := cmd.Flags().Changed("drag-y")

	if hasDragX || hasDragY {
		if len(args) != 1 {
			cli.Fatal("Usage: pinchtab drag <selector> --drag-x <n> --drag-y <n>")
		}
		body := map[string]any{"kind": bridge.ActionDrag}
		setSelectorBody(body, args[0])
		dx, _ := cmd.Flags().GetInt("drag-x")
		dy, _ := cmd.Flags().GetInt("drag-y")
		body["dragX"] = dx
		body["dragY"] = dy
		if button, _ := cmd.Flags().GetString("button"); button != "" {
			body["button"] = button
		}
		postAction(client, base, token, cmd, body)
		return
	}

	if len(args) != 2 {
		cli.Fatal("Usage: pinchtab drag <from> <to>  or  pinchtab drag <selector> --drag-x <n> --drag-y <n>")
	}

	from := parseDragTarget(args[0])
	to := parseDragTarget(args[1])

	mouseDown := map[string]any{"kind": bridge.ActionMouseDown}
	mouseUp := map[string]any{"kind": bridge.ActionMouseUp}
	if button, _ := cmd.Flags().GetString("button"); button != "" {
		mouseDown["button"] = button
		mouseUp["button"] = button
	}

	postAction(client, base, token, cmd, actionBodyForTarget(bridge.ActionMouseMove, from))
	postAction(client, base, token, cmd, mouseDown)
	postAction(client, base, token, cmd, actionBodyForTarget(bridge.ActionMouseMove, to))
	postAction(client, base, token, cmd, mouseUp)
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
		// Precedence: integer pixels > direction keyword > unified selector.
		// Pixels and directions are short, low-cardinality inputs that would
		// otherwise also parse as CSS tag selectors (e.g. "up" / "down"), so
		// we intercept them before handing off to setSelectorBody.
		if px, err := strconv.Atoi(args[0]); err == nil {
			body["scrollY"] = px
			break
		}
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
			// Fall back to the unified selector parser so refs ("e5"),
			// CSS ("#footer", ".class"), XPath ("//..."), text: and
			// semantic selectors all work — same contract as `click`,
			// `fill`, `hover`, etc. Server supports these via req.Selector.
			setSelectorBody(body, args[0])
		}
	case "select":
		setSelectorBody(body, args[0])
		body["value"] = args[1]
	case "keyboard-type":
		body["text"] = strings.Join(args, " ")
	case "keyboard-inserttext":
		body["text"] = strings.Join(args, " ")
	case "keydown":
		body["key"] = args[0]
	case "keyup":
		body["key"] = args[0]
	}

	postAction(client, base, token, cmd, body)
}
