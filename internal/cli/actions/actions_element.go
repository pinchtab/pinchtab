package actions

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/scroll"
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
		body["selector"] = css
	} else if selectorArg != "" {
		setSelectorBody(body, selectorArg)
	} else if !hasXY {
		cli.Fatal("Usage: pinchtab %s <selector> or pinchtab %s --css <selector> or pinchtab %s --x <num> --y <num>", kind, kind, kind)
	}

	if kind == "click" {
		if submit, _ := cmd.Flags().GetBool("submit"); submit {
			body["submit"] = true
		}
		if dismiss, _ := cmd.Flags().GetBool("dismiss-known-interstitials"); dismiss {
			body["dismissKnownInterstitials"] = true
		}
		waitNav, _ := cmd.Flags().GetBool("wait-nav")
		if waitNav {
			body["waitNav"] = true
		}
		mode, _ := cmd.Flags().GetString("mode")
		if mode != "" {
			body["mode"] = mode
		}
		if mode != "" && cmd.Flags().Changed("humanize") {
			if humanize, _ := cmd.Flags().GetBool("humanize"); humanize {
				cli.Fatal("Error: mode and humanize are mutually exclusive")
			}
		}
		// --dismiss-banners only fires when --wait-nav is set; without nav,
		// banners haven't changed and the dismissal pass would be wasted.
		if waitNav {
			if v, _ := cmd.Flags().GetBool("dismiss-banners"); v {
				body["dismissBanners"] = true
			}
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

	// --humanize lets the caller opt into the bezier+jitter input path on
	// any pointer/typing action. Honours the same precedence as the API
	// flag (per-request override > instance config > built-in default).
	// Only set the body field when the flag was explicitly provided so
	// nil/null is preserved when omitted.
	if cmd.Flags().Changed("humanize") {
		v, _ := cmd.Flags().GetBool("humanize")
		body["humanize"] = v
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
		body["selector"] = sel.Value
	default:
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
		path = "/tabs/" + tabID + "/action"
	}

	if _, ok := body["vocab"]; !ok {
		if tok := apiclient.VocabTokenFor(base, tabID); tok != "" {
			body["vocab"] = tok
		}
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPostWithHeaders(client, base, token, path, body, headers)
		return
	}

	result := apiclient.DoPostQuietWithHeaders(client, base, token, path, body, headers)
	kind, _ := body["kind"].(string)
	printActionResult(kind, result)

	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	if snap || snapDiff {
		fetchAndPrintSnapshot(client, base, token, tabID, snapDiff)
	}

	text, _ := cmd.Flags().GetBool("text")
	if text {
		fetchAndPrintText(client, base, token, tabID)
	}
}

func fetchAndPrintSnapshot(client *http.Client, base, token, tabID string, diff bool) {
	params := "filter=interactive&format=compact"
	if diff {
		params += "&diff=true"
	}
	if tabID != "" {
		params += "&tabId=" + tabID
	}
	apiclient.DoGetRawAndPrint(client, base, token, "/snapshot?"+params)
}

func fetchAndPrintText(client *http.Client, base, token, tabID string) {
	path := "/text"
	if tabID != "" {
		path = "/tabs/" + tabID + "/text"
	}
	body := apiclient.DoGetRaw(client, base, token, path, nil)
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	output.Value(result.Text)
}

func printActionResult(kind string, result map[string]any) {
	if success, ok := result["success"].(bool); ok && !success {
		errMsg := "unknown error"
		if msg, ok := result["error"].(string); ok {
			errMsg = msg
		}
		if recovery, ok := result["recovery"].(map[string]any); ok {
			if failType, ok := recovery["failure_type"].(string); ok {
				if failType == "stale" || failType == "navigation" {
					output.Hint("ref may be stale — run `pinchtab snap -i` to refresh")
				}
			}
		}
		output.Error(kind, errMsg, output.ExitNotFound)
		return
	}
	if actionResult, ok := result["result"].(map[string]any); ok {
		// A submit reports its own bounded post-state, which says more than "it
		// moved" — so it is the headline, and the navigation rides underneath it
		// rather than replacing it.
		if postState, ok := actionResult["postState"].(map[string]any); ok {
			status, _ := postState["status"].(string)
			signal, _ := postState["signal"].(string)
			switch status {
			case "pending":
				output.Value("PENDING")
				output.Hint("submit post-state is still pending; do not retry automatically")
				printNavigationOutcome(actionResult)
				return
			case "succeeded":
				output.Value("SUCCEEDED " + signal)
				printNavigationOutcome(actionResult)
				return
			}
		}
		// A click that moved the page succeeded. It used to exit 1 with a 409, which
		// is the signal every agent loop and CI harness branches on, so the natural
		// reaction — retry — re-clicked on the page the first click had reached.
		if actionResult["navigated"] == true {
			landed, _ := actionResult["url"].(string)
			output.Value("OK navigated " + landed)
			printStaleRefsHint()
			return
		}
	}

	output.Success()
}

// printNavigationOutcome reports the landing for an action whose headline is
// something else. Every form that navigates says where it landed, including the
// forms that declare the navigation — those are the ones whose next action depends
// on the new page, so they are the ones that most need to hear their refs are dead.
func printNavigationOutcome(actionResult map[string]any) {
	if actionResult["navigated"] != true {
		return
	}
	if landed, _ := actionResult["url"].(string); landed != "" {
		output.Value("navigated " + landed)
	}
	printStaleRefsHint()
}

func printStaleRefsHint() {
	output.Hint("every ref from your last snapshot is dead — run `pinchtab snap -i` before the next action")
}

func setPointBody(body map[string]any, x, y float64) {
	body["x"] = x
	body["y"] = y
}

// applyScrollTarget fills a scroll body from the single positional — integer pixels, then
// direction keyword, then unified selector — or from --dy/--dx when there is no positional.
// Pixels and directions would otherwise parse as CSS tag selectors ("up", "down"), so they
// are intercepted before setSelectorBody.
//
// A negative count is only reachable through the flags: cobra reads a leading minus on a
// positional as bundled shorthand flags.
//
// The positional and the flags are two spellings of ONE argument, so a positional wins
// outright instead of merging: assigning both would build a diagonal scroll out of a flag
// axis and a positional axis that no caller asked for. The CLI refuses that combination
// before it gets here, but this is exported and callable without cobra's Args hook, so the
// rule lives here rather than on loan from another package.
func applyScrollTarget(body map[string]any, args []string, cmd *cobra.Command) {
	if len(args) == 0 {
		if deltaY, ok := readIntFlag(cmd, "dy"); ok {
			body["scrollY"] = deltaY
		}
		if deltaX, ok := readIntFlag(cmd, "dx"); ok {
			body["scrollX"] = deltaX
		}
		return
	}
	if px, err := strconv.Atoi(args[0]); err == nil {
		body["scrollY"] = px
		return
	}
	if direction, ok := scroll.DirectionFor(strings.ToLower(args[0])); ok {
		body[direction.Axis] = direction.Delta
		return
	}
	setSelectorBody(body, args[0])
}

func readIntFlag(cmd *cobra.Command, primary string) (int, bool) {
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
		if deltaX, ok := readIntFlag(cmd, "dx"); ok {
			body["deltaX"] = deltaX
		}
		if deltaY, ok := readIntFlag(cmd, "dy"); ok {
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

	if cmd.Flags().Changed("humanize") {
		v, _ := cmd.Flags().GetBool("humanize")
		body["humanize"] = v
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

	body := actionBodyForTarget(bridge.ActionDrag, parseDragTarget(args[0]))
	setDragDestinationBody(body, parseDragTarget(args[1]))
	if button, _ := cmd.Flags().GetString("button"); button != "" {
		body["button"] = button
	}
	postAction(client, base, token, cmd, body)
}

func setDragDestinationBody(body map[string]any, target dragTarget) {
	if target.hasXY {
		body["toX"] = target.x
		body["toY"] = target.y
		return
	}
	body["toSelector"] = target.selector
}

func ActionSimple(client *http.Client, base, token, kind string, args []string, cmd *cobra.Command) {
	body := map[string]any{"kind": kind}

	switch kind {
	case "type":
		setSelectorBody(body, args[0])
		body["text"] = strings.Join(args[1:], " ")
	case "fill":
		setSelectorBody(body, args[0])
		body["text"] = strings.Join(args[1:], " ")
		if submit, _ := cmd.Flags().GetBool("submit"); submit {
			body["submit"] = true
		}
	case "press":
		if len(args) >= 2 {
			setSelectorBody(body, args[0])
			body["key"] = args[1]
		} else {
			body["key"] = args[0]
		}
	case "scroll":
		applyScrollTarget(body, args, cmd)
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

	if cmd.Flags().Changed("humanize") {
		v, _ := cmd.Flags().GetBool("humanize")
		body["humanize"] = v
	}

	postAction(client, base, token, cmd, body)
}
