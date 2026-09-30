package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/scroll"
)

// humanizeAction is the set of pointer actions that accept a per-request humanize
// override, mirroring the CLI's addPointerActionFlags set. Only these tools
// declare humanize in their schema; focus is not a pointer action, and only click
// also has mode, so only click can hit the mutual-exclusion rule.
var humanizeAction = map[string]bool{"click": true, "hover": true}

// xyAction is the set of kinds that can be targeted by coordinate, mirroring the
// tools that declare x/y. The bridge honours x/y only for these kinds — the text
// and form actions target by nodeId or selector — so reading it for every kind
// forwarded an inert, undiscoverable and unvalidated pair, hasXY included.
var xyAction = map[string]bool{"click": true, "hover": true, "scroll": true}

// suppliedStringArg resolves an argument whose PRESENCE, not emptiness, says it was asked
// for, and refuses with a message the caller can act on. Both refusals exist because the
// collapsing accessor answers "missing" to two requests that are not missing: one that
// supplied the empty string on purpose, and one that supplied the argument under a type the
// library never enforced. An agent told to send what it just sent has nowhere to go.
//
// emptyMeans says what the empty string DOES on this verb; a verb where it means nothing
// passes "" and the refusals leave the clause out.
func suppliedStringArg(r mcp.CallToolRequest, verb, arg, emptyMeans string, keys ...string) (string, *mcp.CallToolResult) {
	value, supplied, wrongType := firstSuppliedString(r, keys...)
	if supplied {
		return value, nil
	}
	if wrongType != "" {
		return "", wrongTypeRefusal(verb, arg, wrongType, emptyMeans)
	}
	message := fmt.Sprintf("%s needs a '%s' argument", verb, arg)
	if emptyMeans != "" {
		message += "; " + emptyMeans
	}
	return "", mcp.NewToolResultError(message)
}

// wrongTypeRefusal is the one builder for the supplied-under-the-wrong-type refusal, shared
// by the value/text arguments and the selector aliases so the verbs cannot drift on the
// message or lose the remedy. arg names the key the caller actually sent.
func wrongTypeRefusal(verb, arg, gotType, emptyMeans string) *mcp.CallToolResult {
	message := fmt.Sprintf("%s's '%s' must be a string, got %s; quote it", verb, arg, gotType)
	if emptyMeans != "" {
		message += ", or " + emptyMeans
	}
	return mcp.NewToolResultError(message)
}

func handleAction(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		req := bridge.ActionRequest{Kind: kind, TabID: optString(r, "tabId")}
		if xyAction[kind] {
			req.X, req.Y, req.HasXY = resolveXY(r)
		}
		if nodeID, ok := optInt(r, "nodeId"); ok && nodeID > 0 {
			req.NodeID = int64(nodeID)
		}
		if humanizeAction[kind] {
			readHumanize(&req, r)
		}

		var refusal *mcp.CallToolResult
		switch kind {
		case "click":
			refusal = buildClick(&req, r)
		case "hover":
			refusal = requirePointerTarget(&req, r)
		case "focus":
			refusal = requireSelector(&req, r)
		case "type":
			refusal = buildType(&req, r)
		case "select":
			refusal = buildSelect(&req, r)
		case "scroll":
			refusal = buildScroll(&req, r)
		case "scrollintoview":
			refusal = requireSelector(&req, r)
		case "fill":
			refusal = buildFill(&req, r)
		}
		if refusal != nil {
			return refusal, nil
		}

		req.Vocab = c.VocabToken(req.TabID)
		req.Browser = routingBrowser(r)
		body, code, err := c.Post(ctx, routedPath(r, "/action"), req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code < 400 && snapSupportedKinds[kind] {
			return withOptionalSnapshot(ctx, c, r, req.TabID, body, code)
		}
		return resultFromBytes(body, code)
	}
}

// resolveSelector refuses a mistyped selector even when none is required: acting on nodeId
// or query while silently discarding a ref the caller sent is the wrong-target hazard.
func resolveSelector(req *bridge.ActionRequest, r mcp.CallToolRequest, required bool) *mcp.CallToolResult {
	sel, wrongKey, wrongType := actionSelectorArg(r)
	if wrongType != "" {
		return wrongTypeRefusal(req.Kind, wrongKey, wrongType, "")
	}
	if sel != "" {
		req.Selector = sel
		return nil
	}
	if required {
		return mcp.NewToolResultError("required parameter 'selector' is missing")
	}
	return nil
}

func requireSelector(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	return resolveSelector(req, r, req.NodeID == 0)
}

func requirePointerTarget(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	return resolveSelector(req, r, req.NodeID == 0 && !req.HasXY)
}

// readHumanize forwards humanize only when sent, so an omitted one inherits the instance
// default; an explicit false is a real per-request opt-out.
func readHumanize(req *bridge.ActionRequest, r mcp.CallToolRequest) {
	if humanize, ok := optBool(r, "humanize"); ok {
		req.Humanize = &humanize
	}
}

// buildClick exposes the click fields an agent can act on. button is deliberately absent:
// the click action never reads it, so declaring it would advertise an inert argument.
func buildClick(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	if refusal := requirePointerTarget(req, r); refusal != nil {
		return refusal
	}
	req.WaitNav, _ = optBool(r, "waitNav")
	req.Submit, _ = optBool(r, "submit")
	req.DismissKnownInterstitials, _ = optBool(r, "dismissKnownInterstitials")
	req.DismissBanners, _ = optBool(r, "dismissBanners")
	if rawMode := optTrimmedString(r, "mode"); rawMode != "" {
		mode, err := bridge.ParseClickMode(rawMode)
		if err != nil {
			return mcp.NewToolResultError(err.Error())
		}
		if req.Humanize != nil && *req.Humanize {
			return mcp.NewToolResultError("mode and humanize are mutually exclusive")
		}
		req.Mode = mode
	}
	if rawDialogAction := firstNonEmptyString(r, "dialogAction", "onDialog"); rawDialogAction != "" {
		dialogAction, err := bridge.ParseDialogAction("dialogAction", rawDialogAction)
		if err != nil {
			return mcp.NewToolResultError(err.Error())
		}
		req.DialogAction = dialogAction
		req.DialogText = firstNonEmptyString(r, "dialogText", "promptText")
	}
	return nil
}

// buildType keeps collapsing empty with absent, since typing nothing is meaningless, but
// only after a wrong-typed argument has been named as one.
func buildType(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	if refusal := requireSelector(req, r); refusal != nil {
		return refusal
	}
	text, refusal := suppliedStringArg(r, "type", "text", "", "text", "value")
	if refusal != nil {
		return refusal
	}
	if strings.TrimSpace(text) == "" {
		return mcp.NewToolResultError("type needs a non-empty 'text' argument")
	}
	req.Text = text
	return nil
}

// buildSelect treats an empty value as the `<option value="">` placeholder, a real
// selection the bridge tells apart from an absent one through HasText.
func buildSelect(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	if refusal := requireSelector(req, r); refusal != nil {
		return refusal
	}
	value, refusal := suppliedStringArg(r, "select", "value", `send "" to select an empty-valued option`, "value", "option")
	if refusal != nil {
		return refusal
	}
	req.Value, req.HasText = value, true
	return nil
}

// buildFill posts under Text, the field fill reads, and keeps an empty value because that
// is how a caller clears a field.
func buildFill(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	if refusal := requireSelector(req, r); refusal != nil {
		return refusal
	}
	value, refusal := suppliedStringArg(r, "fill", "value", `send "" to clear the field`, "value", "text")
	if refusal != nil {
		return refusal
	}
	req.Text, req.HasText = value, true
	return nil
}

func buildScroll(req *bridge.ActionRequest, r mcp.CallToolRequest) *mcp.CallToolResult {
	if refusal := resolveSelector(req, r, false); refusal != nil {
		return refusal
	}
	if direction := strings.ToLower(optTrimmedString(r, "direction")); direction != "" {
		return buildDirectionScroll(req, r, direction)
	}
	pixels, hasPixels := optInt(r, "pixels")
	deltaX, hasDeltaX := optInt(r, "deltaX")
	deltaY, hasDeltaY := optInt(r, "deltaY")
	if req.HasXY || hasDeltaX || hasDeltaY || (req.Selector != "" && hasPixels) {
		req.Kind = "mouse-wheel"
		if !hasDeltaY && hasPixels {
			deltaY, hasDeltaY = pixels, true
		}
		req.DeltaX, req.DeltaY, req.HasDelta = deltaX, deltaY, hasDeltaX || hasDeltaY
		return nil
	}
	if hasPixels {
		req.ScrollY, req.HasScroll = pixels, true
	}
	return nil
}

// buildDirectionScroll reads the keyword's vocabulary and step from the table the CLI shares.
// It refuses an explicit delta alongside the keyword rather than silently picking one, honours
// a supplied pixels:0 so the shared zero-delta resolver refuses it, and routes an element
// target through the wheel: a page scroll with a target short-circuits into scroll-into-view
// and discards the direction.
func buildDirectionScroll(req *bridge.ActionRequest, r mcp.CallToolRequest, direction string) *mcp.CallToolResult {
	resolved, ok := scroll.DirectionFor(direction)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("direction must be one of %s", strings.Join(scroll.DirectionKeywords(), ", ")))
	}
	_, hasDeltaX := optInt(r, "deltaX")
	_, hasDeltaY := optInt(r, "deltaY")
	if hasDeltaX || hasDeltaY {
		return mcp.NewToolResultError(fmt.Sprintf(
			"direction and deltaX/deltaY each set a scroll magnitude; send one — direction for a %s step, or an explicit delta for a precise wheel move",
			scrollStepDescription))
	}
	steps, hasSteps := optInt(r, "steps")
	if !hasSteps || steps < 1 {
		steps = 1
	}
	distance := resolved.Delta * steps
	if pixels, hasPixels := optInt(r, "pixels"); hasPixels {
		distance = abs(pixels) * steps
		if resolved.Delta < 0 {
			distance = -distance
		}
	}
	if req.Selector != "" || req.NodeID > 0 {
		req.Kind = "mouse-wheel"
	}
	if resolved.Axis == "scrollX" {
		req.ScrollX = distance
	} else {
		req.ScrollY = distance
	}
	req.HasScroll = true
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type keyboardAction struct {
	kind string
	arg  string
}

var keyboardActions = map[string]keyboardAction{
	"press":  {kind: "press", arg: "key"},
	"down":   {kind: "keydown", arg: "key"},
	"up":     {kind: "keyup", arg: "key"},
	"type":   {kind: "keyboard-type", arg: "text"},
	"insert": {kind: "keyboard-inserttext", arg: "text"},
}

func handleKeyboard(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, action, refusal := pickMode(r, "action", keyboardActions)
		if refusal != nil {
			return refusal, nil
		}
		value, err := r.RequireString(action.arg)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"kind": action.kind, action.arg: value}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		if nodeID, ok := optInt(r, "nodeId"); ok && nodeID > 0 {
			payload["nodeId"] = nodeID
		}
		return toolResult(c.Post(ctx, routedPathWithBody(r, "/action", payload), payload))
	}
}

// snapSupportedKinds are the interaction kinds whose tools declare snap. Only
// these follow the action with a snapshot; mcp-go does not reject an undeclared
// argument, so a snap sent to hover or scroll must not silently change what the
// tool returns.
var snapSupportedKinds = map[string]bool{"click": true, "fill": true, "select": true}
