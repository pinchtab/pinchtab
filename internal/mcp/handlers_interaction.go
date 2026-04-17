package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func handleAction(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]any{"kind": kind}

		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}

		x, y, hasXY := resolveXY(r)
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

		resolveSelector := func(required bool) (bool, error) {
			sel := actionSelectorArg(r)
			if sel != "" {
				payload["selector"] = sel
				return true, nil
			}
			if required {
				return false, fmt.Errorf("required parameter 'selector' is missing")
			}
			return false, nil
		}

		switch kind {
		case "click", "hover", "focus":
			requiresSelector := !hasNodeID
			if kind == "click" || kind == "hover" {
				requiresSelector = requiresSelector && !hasXY
			}
			if _, err := resolveSelector(requiresSelector); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if kind == "click" {
				if waitNav, ok := optBool(r, "waitNav"); ok && waitNav {
					payload["waitNav"] = true
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
			if _, err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			text := firstNonEmptyString(r, "text", "value")
			if text == "" {
				return mcp.NewToolResultError("required parameter 'text' is missing"), nil
			}
			payload["text"] = text

		case "press":
			key, err := r.RequireString("key")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload["key"] = key

		case "select":
			if _, err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			value := firstNonEmptyString(r, "value", "option")
			if value == "" {
				return mcp.NewToolResultError("required parameter 'value' is missing"), nil
			}
			payload["value"] = value

		case "scroll":
			hasSelector, err := resolveSelector(false)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pixels, hasPixels := optInt(r, "pixels")
			deltaX, hasDeltaX := optInt(r, "deltaX")
			deltaY, hasDeltaY := optInt(r, "deltaY")

			direction := strings.ToLower(optTrimmedString(r, "direction"))
			steps, hasSteps := optInt(r, "steps")
			if !hasSteps || steps < 1 {
				steps = 1
			}

			if direction != "" && !hasDeltaY {
				magnitude := 120
				if hasPixels && pixels != 0 {
					magnitude = pixels
					if magnitude < 0 {
						magnitude = -magnitude
					}
				}
				magnitude *= steps
				switch direction {
				case "down":
					deltaY = magnitude
				case "up":
					deltaY = -magnitude
				default:
					return mcp.NewToolResultError("direction must be 'up' or 'down'"), nil
				}
				hasDeltaY = true
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
			if _, err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

		case "fill":
			if _, err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			value := firstNonEmptyString(r, "value", "text")
			if value == "" {
				return mcp.NewToolResultError("required parameter 'value' is missing"), nil
			}
			payload["value"] = value
		}

		body, code, err := c.Post(ctx, "/action", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleKeyboardText(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := r.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"kind": kind, "text": text}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/action", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleKeyboardKey(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := r.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"kind": kind, "key": key}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/action", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}
