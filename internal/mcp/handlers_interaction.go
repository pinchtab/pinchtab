package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
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
		payload := map[string]any{"kind": kind}

		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}

		x, y, hasXY := 0.0, 0.0, false
		if xyAction[kind] {
			x, y, hasXY = resolveXY(r)
		}
		if hasXY {
			payload["x"] = x
			payload["y"] = y
			payload["hasXY"] = true
		}

		hasNodeID := false
		if nodeID, ok := optInt(r, "nodeId"); ok && nodeID > 0 {
			hasNodeID = true
			payload["nodeId"] = nodeID
		}

		resolveSelector := func(required bool) (bool, *mcp.CallToolResult) {
			sel, wrongKey, wrongType := actionSelectorArg(r)
			if wrongType != "" {
				// Refused even when a selector is not required: acting on nodeId or
				// query while silently discarding a mistyped ref the caller sent is
				// the wrong-target hazard, not a convenience.
				return false, wrongTypeRefusal(kind, wrongKey, wrongType, "")
			}
			if sel != "" {
				payload["selector"] = sel
				return true, nil
			}
			if required {
				return false, mcp.NewToolResultError("required parameter 'selector' is missing")
			}
			return false, nil
		}

		switch kind {
		case "click", "hover", "focus":
			requiresSelector := !hasNodeID
			if kind == "click" || kind == "hover" {
				requiresSelector = requiresSelector && !hasXY
			}
			if _, refusal := resolveSelector(requiresSelector); refusal != nil {
				return refusal, nil
			}
			// Forwarded only when the caller actually sent it, so an omitted
			// humanize inherits the instance default instead of overriding it for
			// every request. An explicit false is a real opt-out and travels: the
			// wire field is a per-request override, not a flag.
			humanize, humanizeSet := false, false
			if humanizeAction[kind] {
				humanize, humanizeSet = optBool(r, "humanize")
				if humanizeSet {
					payload["humanize"] = humanize
				}
			}
			if kind == "click" {
				if waitNav, ok := optBool(r, "waitNav"); ok && waitNav {
					payload["waitNav"] = true
				}
				if mode := strings.ToLower(strings.TrimSpace(optTrimmedString(r, "mode"))); mode != "" {
					if mode != "dom" && mode != "dispatch" {
						return mcp.NewToolResultError("mode must be 'dom' or 'dispatch'"), nil
					}
					if humanizeSet && humanize {
						return mcp.NewToolResultError("mode and humanize are mutually exclusive"), nil
					}
					payload["mode"] = mode
				}
				dialogAction := strings.ToLower(firstNonEmptyString(r, "dialogAction", "onDialog"))
				if dialogAction != "" {
					if dialogAction != "accept" && dialogAction != "dismiss" {
						return mcp.NewToolResultError("dialogAction must be 'accept' or 'dismiss'"), nil
					}
					payload["dialogAction"] = dialogAction
					if dialogText := firstNonEmptyString(r, "dialogText", "promptText"); dialogText != "" {
						payload["dialogText"] = dialogText
					}
				}
			}

		case "type":
			if _, refusal := resolveSelector(!hasNodeID); refusal != nil {
				return refusal, nil
			}
			// Typing nothing is meaningless, so type keeps collapsing empty with absent —
			// but only after a wrong-typed argument has been named as one, rather than
			// reported as an argument the caller never sent.
			text, refusal := suppliedStringArg(r, "type", "text", "", "text", "value")
			if refusal != nil {
				return refusal, nil
			}
			if strings.TrimSpace(text) == "" {
				return mcp.NewToolResultError("type needs a non-empty 'text' argument"), nil
			}
			payload["text"] = text

		case "select":
			if _, refusal := resolveSelector(!hasNodeID); refusal != nil {
				return refusal, nil
			}
			// `<option value="">` is the standard placeholder, so an empty value is a real
			// selection — the one that resets a dropdown — and the bridge tells it apart
			// from an absent one by key presence.
			value, refusal := suppliedStringArg(r, "select", "value", `send "" to select an empty-valued option`, "value", "option")
			if refusal != nil {
				return refusal, nil
			}
			payload["value"] = value

		case "scroll":
			hasSelector, refusal := resolveSelector(false)
			if refusal != nil {
				return refusal, nil
			}
			pixels, hasPixels := optInt(r, "pixels")
			deltaX, hasDeltaX := optInt(r, "deltaX")
			deltaY, hasDeltaY := optInt(r, "deltaY")

			direction := strings.ToLower(optTrimmedString(r, "direction"))
			steps, hasSteps := optInt(r, "steps")
			if !hasSteps || steps < 1 {
				steps = 1
			}

			// A direction keyword is an INTENT, so it reads its vocabulary and its
			// magnitude from the one owner both surfaces share and routes to the same
			// `scroll` action the CLI's keyword does. It used to carry its own switch
			// (down and up only, no horizontal) and its own bare 120 — a wheel notch,
			// which is a device unit: "down" moved a sixth of what the CLI moved, took
			// six round trips to bring the next content into view, and said so nowhere.
			if direction != "" {
				resolved, ok := scroll.DirectionFor(direction)
				if !ok {
					return mcp.NewToolResultError(fmt.Sprintf("direction must be one of %s", strings.Join(scroll.DirectionKeywords(), ", "))), nil
				}
				// Two magnitudes and no rule that picks between them. The keyword used to be
				// dropped here without a word, which is the silent reinterpretation this tool
				// is being fixed to stop — so it is refused instead, naming both spellings.
				if hasDeltaX || hasDeltaY {
					return mcp.NewToolResultError(fmt.Sprintf(
						"direction and deltaX/deltaY each set a scroll magnitude; send one — direction for a %s step, or an explicit delta for a precise wheel move",
						scrollStepDescription)), nil
				}
				// Presence, not truthiness. hasPixels already says the caller supplied a
				// magnitude, and re-testing it for non-zero threw that away: a computed
				// pixels:0 fell through to the keyword step and scrolled a full step in the
				// direction the caller named, from a caller asking for no movement. Honouring
				// the zero routes it into the shared zero-delta resolver, which already
				// refuses it for every other spelling — so the rule keeps one owner and this
				// caller stops being the one that never reaches it.
				distance := resolved.Delta * steps
				if hasPixels {
					magnitude := pixels
					if magnitude < 0 {
						magnitude = -magnitude
					}
					distance = magnitude * steps
					if resolved.Delta < 0 {
						distance = -distance
					}
				}
				// An element target means the caller is aiming the keyword INSIDE something —
				// an inner scrollable container — so it rides the wheel at that element, the
				// same routing an explicit magnitude gets. Posting it as a page scroll instead
				// reaches actionScroll, which short-circuits on a target into scroll-into-view
				// and discards the direction, the distance and the sign while still answering
				// scrolled:true. A coordinate target needs no such move: actionScroll honours
				// x/y and the delta together.
				if hasSelector || hasNodeID {
					payload["kind"] = "mouse-wheel"
				}
				// scrollX/scrollY is the wheel's legacy spelling, so the axis mapping stays
				// owned by the direction table on both routes.
				payload[resolved.Axis] = distance
				break
			}

			useWheel := hasXY || hasDeltaX || hasDeltaY || (hasSelector && hasPixels)
			if useWheel {
				payload["kind"] = "mouse-wheel"
				if hasDeltaX {
					payload["deltaX"] = deltaX
				}
				if hasDeltaY {
					payload["deltaY"] = deltaY
				} else if hasPixels {
					payload["deltaY"] = pixels
				}
			} else if hasPixels {
				payload["scrollY"] = pixels
			}

		case "scrollintoview":
			if _, refusal := resolveSelector(!hasNodeID); refusal != nil {
				return refusal, nil
			}

		case "fill":
			if _, refusal := resolveSelector(!hasNodeID); refusal != nil {
				return refusal, nil
			}
			// Presence, not emptiness: an empty value is how a caller clears a field,
			// so refusing it made the one documented clear idiom inexpressible here
			// while the raw API accepted it — and the old message claimed a parameter
			// the caller had supplied was missing.
			value, refusal := suppliedStringArg(r, "fill", "value", `send "" to clear the field`, "value", "text")
			if refusal != nil {
				return refusal, nil
			}
			// "text" is the field actionFill reads. Posting the same string under "value"
			// reached a real ActionRequest field that fill ignores, so the write was empty
			// and the tool still answered filled:true.
			payload["text"] = value
		}

		if tok := c.VocabToken(optString(r, "tabId")); tok != "" {
			payload["vocab"] = tok
		}

		body, code, err := c.Post(ctx, routedPathWithBody(r, "/action", payload), payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}

		if snapSupportedKinds[kind] {
			return withOptionalSnapshot(ctx, c, r, optString(r, "tabId"), body, code)
		}
		return resultFromBytes(body, code)
	}
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
		body, code, err := c.Post(ctx, routedPathWithBody(r, "/action", payload), payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// snapSupportedKinds are the interaction kinds whose tools declare snap. Only
// these follow the action with a snapshot; mcp-go does not reject an undeclared
// argument, so a snap sent to hover or scroll must not silently change what the
// tool returns.
var snapSupportedKinds = map[string]bool{"click": true, "fill": true, "select": true}
