package bridge

import (
	"context"
	"encoding/json"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/engine"
)

// Text echoes: /action, /actions and /macro used to return the literal string
// that was typed or filled ({"typed": "<secret>"}). When the target is a
// password or otherwise sensitive field, that plaintext lands in the caller's
// transcript and in every log sink downstream. See SMI-3579.
//
// The helpers here replace the literal with a redacted descriptor
// ({"typed_len": N, "redacted": true}) whenever the target field looks
// sensitive, and keep the plaintext echo otherwise so existing callers on
// ordinary form fields are unaffected.

const (
	echoKeyTyped  = "typed"
	echoKeyFilled = "filled"
)

// redactedEcho is the result map returned in place of a plaintext echo.
func redactedEcho(key, text string, extra map[string]any) map[string]any {
	out := map[string]any{
		key + "_len": len([]rune(text)),
		"redacted":   true,
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// plainEcho keeps the historical shape for non-sensitive fields.
func plainEcho(key, text string, extra map[string]any) map[string]any {
	out := map[string]any{key: text}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// textEcho builds the result map for a text-entry action, redacting the echo
// when the target element is sensitive. target selects how the element is
// resolved: a CSS selector, a backend node id, or (when both are empty/zero)
// the currently focused element — which is the correct target for the
// keyboard-type paths that operate on focus.
func textEcho(ctx context.Context, selector string, nodeID int64, key, text string, extra map[string]any) map[string]any {
	if fieldIsSensitive(ctx, selector, nodeID) {
		return redactedEcho(key, text, extra)
	}
	return plainEcho(key, text, extra)
}

// RedactedTextEcho is the redacted echo for callers outside this package
// (the lite engine path in handlers). It never returns the plaintext.
func RedactedTextEcho(text string) map[string]any {
	return redactedEcho(echoKeyTyped, text, nil)
}

const sensitiveProbeJS = `function() {
	var el = this;
	if (!el) { return null; }
	return {
		type: el.getAttribute ? (el.getAttribute('type') || '') : '',
		autocomplete: el.getAttribute ? (el.getAttribute('autocomplete') || '') : '',
		name: el.getAttribute ? (el.getAttribute('name') || '') : '',
		id: el.id || '',
		ariaLabel: el.getAttribute ? (el.getAttribute('aria-label') || '') : ''
	};
}`

type fieldAttrs struct {
	Type         string `json:"type"`
	Autocomplete string `json:"autocomplete"`
	Name         string `json:"name"`
	ID           string `json:"id"`
	AriaLabel    string `json:"ariaLabel"`
}

// fieldIsSensitive probes the target element's attributes. On any resolution
// failure it fails closed (treats the field as sensitive) so a probe error can
// never turn into a plaintext leak.
func fieldIsSensitive(ctx context.Context, selector string, nodeID int64) bool {
	attrs, err := readFieldAttrs(ctx, selector, nodeID)
	if err != nil || attrs == nil {
		return true
	}
	return engine.IsSensitiveFieldAttrs(attrs.Type, attrs.Autocomplete, attrs.Name, attrs.ID, attrs.AriaLabel)
}

func readFieldAttrs(ctx context.Context, selector string, nodeID int64) (*fieldAttrs, error) {
	if nodeID > 0 {
		return readFieldAttrsByNodeID(ctx, nodeID)
	}
	expr := "(function(){var el = document.activeElement;"
	if selector != "" {
		sel, err := json.Marshal(selector)
		if err != nil {
			return nil, err
		}
		expr = "(function(){var el = document.querySelector(" + string(sel) + ") || document.activeElement;"
	}
	expr += "if(!el){return null;} return (" + sensitiveProbeJS + ").call(el);})()"

	var attrs *fieldAttrs
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &attrs)); err != nil {
		return nil, err
	}
	return attrs, nil
}

func readFieldAttrsByNodeID(ctx context.Context, nodeID int64) (*fieldAttrs, error) {
	var attrs *fieldAttrs
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var resolvedRaw json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolvedRaw); err != nil {
			return err
		}
		var resolved struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		if err := json.Unmarshal(resolvedRaw, &resolved); err != nil {
			return err
		}
		var callRaw json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": sensitiveProbeJS,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callRaw); err != nil {
			return err
		}
		var cr struct {
			Result struct {
				Value *fieldAttrs `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(callRaw, &cr); err != nil {
			return err
		}
		attrs = cr.Result.Value
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return attrs, nil
}
