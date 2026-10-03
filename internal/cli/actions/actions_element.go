package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
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

func Action(conn apiclient.Conn, kind, selectorArg string, cmd *cobra.Command) {
	req := bridge.ActionRequest{Kind: kind}

	css, _ := cmd.Flags().GetString("css")
	hasXY := cmd.Flags().Changed("x") || cmd.Flags().Changed("y")
	if hasXY {
		x, _ := cmd.Flags().GetFloat64("x")
		y, _ := cmd.Flags().GetFloat64("y")
		setPoint(&req, x, y)
	}

	req.Button, _ = cmd.Flags().GetString("button")
	if css != "" {
		req.Selector = css
	} else if selectorArg != "" {
		setSelector(&req, selectorArg)
	} else if !hasXY {
		cli.Fatal("Usage: pinchtab %s <selector> or pinchtab %s --css <selector> or pinchtab %s --x <num> --y <num>", kind, kind, kind)
	}

	readHumanizeFlag(&req, cmd)
	if kind == "click" {
		applyClickFlags(&req, cmd)
	}

	postAction(conn, cmd, req)
}

func applyClickFlags(req *bridge.ActionRequest, cmd *cobra.Command) {
	req.Submit, _ = cmd.Flags().GetBool("submit")
	req.DismissKnownInterstitials, _ = cmd.Flags().GetBool("dismiss-known-interstitials")
	req.WaitNav, _ = cmd.Flags().GetBool("wait-nav")
	req.Mode, _ = cmd.Flags().GetString("mode")
	if req.Mode != "" && req.Humanize != nil && *req.Humanize {
		cli.Fatal("Error: mode and humanize are mutually exclusive")
	}
	if req.WaitNav {
		req.DismissBanners, _ = cmd.Flags().GetBool("dismiss-banners")
	}
	req.DialogAction, _ = cmd.Flags().GetString("dialog-action")
	req.DialogText, _ = cmd.Flags().GetString("dialog-text")
}

// readHumanizeFlag forwards --humanize only when given, so an omitted flag inherits the
// instance default instead of overriding it.
func readHumanizeFlag(req *bridge.ActionRequest, cmd *cobra.Command) {
	if cmd.Flags().Changed("humanize") {
		humanize, _ := cmd.Flags().GetBool("humanize")
		req.Humanize = &humanize
	}
}

// setSelectorBody parses a unified selector string and sets the appropriate
// body fields. Ref selectors use the "ref" field; all other kinds use
// "selector" and MUST retain their kind prefix (e.g. `text:Submit`,
// `xpath://button`, `semantic:accept button`) so the server can re-parse
// them correctly. For auto-detected CSS (no prefix on input), the raw
// input is forwarded verbatim.
func setSelector(req *bridge.ActionRequest, s string) {
	sel := selector.Parse(s)
	switch sel.Kind {
	case selector.KindRef:
		req.Ref = sel.Value
	case selector.KindCSS:
		req.Selector = sel.Value
	default:
		req.Selector = s
	}
}

func postAction(conn apiclient.Conn, cmd *cobra.Command, req bridge.ActionRequest) {
	postActionWithHeaders(conn, cmd, req, nil)
}

func postActionWithHeaders(conn apiclient.Conn, cmd *cobra.Command, req bridge.ActionRequest, headers map[string]string) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tabID, "action")

	if req.Vocab == "" {
		if vocabTab, tok := apiclient.VocabForAction(conn.Base, tabID); tok != "" {
			req.Vocab, req.VocabTab = tok, vocabTab
		}
	}

	opts := []apiclient.RequestOption{apiclient.WithJSONBody(req), apiclient.WithHeaders(headers), apiclient.CaptureVocab(namedNoTab(cmd))}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Post(path, nil, opts...)
		return
	}

	result := conn.PostQuiet(path, nil, opts...)
	printActionResult(req.Kind, result)
	if openedDialog(result) != nil {
		return
	}
	if opened := openedTab(result); opened != "" {
		tabID = opened
	}

	if tailRequested(cmd) {
		settleNetwork(conn, tabID)
		printPostActionOutput(conn, tabID, cmd)
	}
}

const settleTimeoutMs = 1500

// settleNetwork holds the --snap / --snap-diff / --text tail until requests the
// action started have gone quiet, capped at settleTimeoutMs. A tab without
// network capture settles immediately and silently; hitting the cap prints a
// settle header so the read is not mistaken for a settled one.
func settleNetwork(conn apiclient.Conn, tabID string) {
	body := map[string]any{"load": "network-idle", "timeout": settleTimeoutMs}
	result := conn.PostQuiet(apiclient.TabPath(tabID, "wait"), body)
	if waited, _ := result["waited"].(bool); waited {
		return
	}
	msg, _ := result["error"].(string)
	if msg == "" || strings.Contains(msg, "network monitor unavailable") {
		return
	}
	fmt.Printf("settle: %s\n", msg)
}

func namedNoTab(cmd *cobra.Command) bool {
	tab, _ := cmd.Flags().GetString("tab")
	return tab == ""
}

// fetchAndPrintSnapshot is the --snap / --snap-diff tail. It stays best-effort —
// a transport or HTTP failure warns on stderr and returns, never exits: this tail
// runs after an action that already succeeded, so a cosmetic snapshot failure
// must not turn a successful action into a non-zero exit.
func fetchAndPrintSnapshot(conn apiclient.Conn, cmd *cobra.Command, tabID string, diff bool) {
	params := url.Values{"filter": {"interactive"}, "format": {"compact"}}
	if diff {
		params.Set("diff", "true")
	}
	if tabID != "" {
		params.Set("tabId", tabID)
	}
	body, err := conn.Raw(http.MethodGet, "/snapshot", apiclient.WithQuery(params), apiclient.CaptureVocab(namedNoTab(cmd)))
	var statusErr *apiclient.StatusError
	switch {
	case errors.As(err, &statusErr):
		fmt.Fprintf(os.Stderr, "snapshot error %d: %s\n", statusErr.Status, string(statusErr.Body))
	case err != nil:
		fmt.Fprintf(os.Stderr, "snapshot failed: %v\n", errors.Unwrap(err))
	default:
		fmt.Println(string(body))
	}
}

func fetchAndPrintText(conn apiclient.Conn, tabID string) {
	path := apiclient.TabPath(tabID, "text")
	body := conn.GetRaw(path, nil)
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	output.Value(result.Text)
}

func openedTab(result map[string]any) string {
	actionResult, _ := result["result"].(map[string]any)
	opened, _ := actionResult["switchedToTab"].(string)
	return strings.TrimSpace(opened)
}

func openedDialog(result map[string]any) map[string]any {
	actionResult, _ := result["result"].(map[string]any)
	dialog, _ := actionResult[bridge.OpenedDialogResultKey].(map[string]any)
	return dialog
}

func printOpenedDialog(result map[string]any) bool {
	dialog := openedDialog(result)
	if dialog == nil {
		return false
	}
	dialogType, _ := dialog["dialogType"].(string)
	message, _ := dialog["dialogMessage"].(string)
	output.Value(fmt.Sprintf("OK opened %s dialog: %q", dialogType, message))
	output.Hint("the action landed and opened a JavaScript dialog — answer it with `pinchtab dialog accept` or `pinchtab dialog dismiss`; do not repeat it")
	return true
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
	if printOpenedDialog(result) {
		return
	}
	if actionResult, ok := result["result"].(map[string]any); ok {
		// A submit reports its own bounded post-state, which says more than "it
		// moved" — so it is the headline, and the navigation rides underneath it
		// rather than replacing it.
		if postState, ok := actionResult["postState"].(map[string]any); ok && printSubmitPostState(postState, actionResult) {
			printOpenedTab(result, "")
			return
		}
		if printOpenedTab(result, "OK ") {
			return
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
		if actionResult["navigated"] == false {
			output.Value("OK no navigation")
			return
		}
		if actionResult["scrolled"] == false {
			output.Value(scrollDidNotMove(actionResult))
			return
		}
	}

	output.Success()
}

func scrollDidNotMove(actionResult map[string]any) string {
	scrollX, _ := actionResult["scrollX"].(float64)
	scrollY, _ := actionResult["scrollY"].(float64)
	return fmt.Sprintf("OK (did not move, scrollX %.0f, scrollY %.0f)", scrollX, scrollY)
}

func printSubmitPostState(postState, actionResult map[string]any) bool {
	status, _ := postState["status"].(string)
	signal, _ := postState["signal"].(string)
	switch status {
	case "pending":
		output.Value("PENDING")
		output.Hint("submit post-state is still pending; do not retry automatically")
		printNavigationOutcome(actionResult)
	case "blocked":
		output.Value("BLOCKED " + signal)
		printInvalidFields(postState["invalidFields"])
	case "succeeded":
		output.Value("SUCCEEDED " + signal)
		if liveRegionText, _ := postState["liveRegionText"].(string); liveRegionText != "" {
			output.Value(liveRegionText)
		}
		printNavigationOutcome(actionResult)
	default:
		return false
	}
	return true
}

func printOpenedTab(result map[string]any, prefix string) bool {
	opened := openedTab(result)
	if opened == "" {
		return false
	}
	output.Value(prefix + "opened tab " + opened)
	output.Hint("the click opened a new tab — run `pinchtab tab " + opened + "` to switch to it")
	return true
}

func printInvalidFields(raw any) {
	fields, _ := raw.([]any)
	for _, f := range fields {
		field, _ := f.(map[string]any)
		name, _ := field["name"].(string)
		label, _ := field["label"].(string)
		message, _ := field["message"].(string)
		subject := name
		if label != "" && label != name {
			subject = strings.TrimSpace(name + " (" + label + ")")
		}
		output.Value(subject + ": " + message)
	}
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

func setPoint(req *bridge.ActionRequest, x, y float64) {
	req.X, req.Y, req.HasXY = x, y, true
}

func setScroll(req *bridge.ActionRequest, axis string, delta int) {
	if axis == "scrollX" {
		req.ScrollX = delta
	} else {
		req.ScrollY = delta
	}
	req.HasScroll = true
}

// applyScrollTarget fills a scroll body from the single positional — integer pixels, then
// direction keyword, then unified selector — or from --dy/--dx when there is no positional.
// Pixels and directions would otherwise parse as CSS tag selectors ("up", "down"), so they
// are intercepted before setSelector.
//
// A negative count is only reachable through the flags: cobra reads a leading minus on a
// positional as bundled shorthand flags.
//
// The positional and the flags are two spellings of ONE argument, so a positional wins
// outright instead of merging: assigning both would build a diagonal scroll out of a flag
// axis and a positional axis that no caller asked for. The CLI refuses that combination
// before it gets here, but this is exported and callable without cobra's Args hook, so the
// rule lives here rather than on loan from another package.
func applyScrollTarget(req *bridge.ActionRequest, args []string, cmd *cobra.Command) {
	if len(args) == 0 {
		if deltaY, ok := readIntFlag(cmd, "dy"); ok {
			setScroll(req, "scrollY", deltaY)
		}
		if deltaX, ok := readIntFlag(cmd, "dx"); ok {
			setScroll(req, "scrollX", deltaX)
		}
		return
	}
	if px, err := strconv.Atoi(args[0]); err == nil {
		setScroll(req, "scrollY", px)
		return
	}
	if direction, ok := scroll.DirectionFor(strings.ToLower(args[0])); ok {
		setScroll(req, direction.Axis, direction.Delta)
		return
	}
	setSelector(req, args[0])
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

func applyMouseTarget(req *bridge.ActionRequest, selectorArg string, cmd *cobra.Command) bool {
	css, _ := cmd.Flags().GetString("css")
	if cmd.Flags().Changed("x") || cmd.Flags().Changed("y") {
		x, _ := cmd.Flags().GetFloat64("x")
		y, _ := cmd.Flags().GetFloat64("y")
		setPoint(req, x, y)
		return true
	}
	if css != "" {
		req.Selector = css
		return true
	}
	if selectorArg != "" {
		setSelector(req, selectorArg)
		return true
	}
	return false
}

func MouseAction(conn apiclient.Conn, kind string, args []string, cmd *cobra.Command) {
	req := bridge.ActionRequest{Kind: kind}
	req.Button, _ = cmd.Flags().GetString("button")

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
			setPoint(&req, x, y)
		} else if len(args) == 1 {
			if cmd.Flags().Changed("x") || cmd.Flags().Changed("y") || cmd.Flags().Changed("css") {
				cli.Fatal("Usage: pinchtab mouse move <x> <y> or pinchtab mouse move <selector> or pinchtab mouse move --x <num> --y <num>")
			}
			setSelector(&req, args[0])
		} else if !applyMouseTarget(&req, "", cmd) {
			cli.Fatal("Usage: pinchtab mouse move <x> <y> or pinchtab mouse move <selector> or pinchtab mouse move --x <num> --y <num>")
		}
	case bridge.ActionMouseDown, bridge.ActionMouseUp:
		if len(args) > 1 {
			cli.Fatal("Usage: pinchtab mouse %s [selector]", strings.TrimPrefix(kind, "mouse-"))
		}
		_ = applyMouseTarget(&req, optionalMouseArg(args), cmd)
	case bridge.ActionMouseWheel:
		if len(args) > 1 {
			cli.Fatal("Usage: pinchtab mouse wheel <dy> [--dx <n>] or pinchtab mouse wheel [selector]")
		}
		deltaYFromArg := false
		if len(args) == 1 {
			if dy, err := strconv.Atoi(args[0]); err == nil {
				req.DeltaY, req.HasDelta, deltaYFromArg = dy, true, true
			} else {
				setSelector(&req, args[0])
			}
		}
		if deltaX, ok := readIntFlag(cmd, "dx"); ok {
			req.DeltaX, req.HasDelta = deltaX, true
		}
		if deltaY, ok := readIntFlag(cmd, "dy"); ok {
			if deltaYFromArg {
				cli.Fatal("Usage: pinchtab mouse wheel <dy> [--dx <n>] or pinchtab mouse wheel [selector]")
			}
			req.DeltaY, req.HasDelta = deltaY, true
		}
		if req.Selector == "" && req.Ref == "" {
			_ = applyMouseTarget(&req, "", cmd)
		} else if cmd.Flags().Changed("x") || cmd.Flags().Changed("y") || cmd.Flags().Changed("css") {
			cli.Fatal("Usage: pinchtab mouse wheel <dy> [--dx <n>] or pinchtab mouse wheel [selector]")
		}
	default:
		cli.Fatal("unsupported mouse action: %s", kind)
	}

	readHumanizeFlag(&req, cmd)
	postAction(conn, cmd, req)
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

func actionRequestForTarget(kind string, target dragTarget) bridge.ActionRequest {
	req := bridge.ActionRequest{Kind: kind}
	if target.hasXY {
		setPoint(&req, target.x, target.y)
		return req
	}
	setSelector(&req, target.selector)
	return req
}

func Drag(conn apiclient.Conn, args []string, cmd *cobra.Command) {
	hasDragX := cmd.Flags().Changed("drag-x")
	hasDragY := cmd.Flags().Changed("drag-y")

	if hasDragX || hasDragY {
		if len(args) != 1 {
			cli.Fatal("Usage: pinchtab drag <selector> --drag-x <n> --drag-y <n>")
		}
		req := bridge.ActionRequest{Kind: bridge.ActionDrag}
		setSelector(&req, args[0])
		req.DragX, _ = cmd.Flags().GetInt("drag-x")
		req.DragY, _ = cmd.Flags().GetInt("drag-y")
		req.Button, _ = cmd.Flags().GetString("button")
		postAction(conn, cmd, req)
		return
	}

	if len(args) != 2 {
		cli.Fatal("Usage: pinchtab drag <from> <to>  or  pinchtab drag <selector> --drag-x <n> --drag-y <n>")
	}

	req := actionRequestForTarget(bridge.ActionDrag, parseDragTarget(args[0]))
	setDragDestination(&req, parseDragTarget(args[1]))
	req.Button, _ = cmd.Flags().GetString("button")
	postAction(conn, cmd, req)
}

func setDragDestination(req *bridge.ActionRequest, target dragTarget) {
	if target.hasXY {
		req.ToX, req.ToY, req.HasToXY = target.x, target.y, true
		return
	}
	req.ToSelector = target.selector
}

func ActionSimple(conn apiclient.Conn, kind string, args []string, cmd *cobra.Command) {
	req := bridge.ActionRequest{Kind: kind}

	switch kind {
	case "type":
		setSelector(&req, args[0])
		setText(&req, strings.Join(args[1:], " "))
	case "fill":
		setSelector(&req, args[0])
		setText(&req, strings.Join(args[1:], " "))
		req.Submit, _ = cmd.Flags().GetBool("submit")
	case "press":
		if len(args) >= 2 {
			setSelector(&req, args[0])
			req.Key = args[1]
		} else {
			req.Key = args[0]
		}
	case "scroll":
		applyScrollTarget(&req, args, cmd)
	case "select":
		setSelector(&req, args[0])
		req.Value, req.HasText = args[1], true
	case "keyboard-type", "keyboard-inserttext":
		setText(&req, strings.Join(args, " "))
	case "keydown", "keyup":
		req.Key = args[0]
	}

	readHumanizeFlag(&req, cmd)
	postAction(conn, cmd, req)
}

func setText(req *bridge.ActionRequest, text string) {
	req.Text, req.HasText = text, true
}
