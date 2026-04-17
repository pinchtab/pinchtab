package cdpops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

func FillByNodeID(ctx context.Context, nodeID int64, value string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var result json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
				"backendNodeId": nodeID,
			}, &result); err != nil {
				return err
			}
			var resolved struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := json.Unmarshal(result, &resolved); err != nil {
				return err
			}
			// Use the native value setter for the concrete element type to bypass
			// framework-patched setters (e.g. React's value tracker). Calling the
			// input setter on a textarea throws with an incompatible receiver.
			js := `function(v) {
				var proto = null;
				if (this instanceof window.HTMLTextAreaElement) {
					proto = window.HTMLTextAreaElement.prototype;
				} else if (this instanceof window.HTMLInputElement) {
					proto = window.HTMLInputElement.prototype;
				}
				var setter = proto && Object.getOwnPropertyDescriptor(proto, 'value').set;
				if (setter) { setter.call(this, v); } else { this.value = v; }
				this.dispatchEvent(new Event('input', {bubbles: true}));
				this.dispatchEvent(new Event('change', {bubbles: true}));
			}`
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
				"functionDeclaration": js,
				"objectId":            resolved.Object.ObjectID,
				"arguments":           []map[string]any{{"value": value}},
			}, nil)
		}),
	)
}

// SelectByNodeID sets the value of a <select> element, with a forgiving
// lookup. Strategies are tried in order; the first match wins:
//
//  1. Exact `<option value="...">` attribute.
//  2. Exact (trimmed) visible text.
//  3. Case-insensitive trimmed visible text.
//  4. Case-insensitive substring of visible text (last resort).
//
// This lets callers pass whatever they have — the canonical `value` attr
// ("uk"), the visible text they see in a snapshot ("United Kingdom"), or an
// obvious shorthand ("Dark" → "Dark Mode"). The substring strategy is
// deliberately permissive; if multiple options share a prefix the first one
// in document order wins, so prefer exact forms when disambiguation matters.
//
// When the element isn't a <select> (no `.options` collection), behavior
// degrades to the original "set .value directly" path — preserves backward
// compatibility for anyone using this helper on a plain input or custom
// element.
//
// Returns an error when none of the three match strategies find an option on
// a <select> element; the error enumerates available values + texts to make
// the failure actionable.
func SelectByNodeID(ctx context.Context, nodeID int64, value string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var result json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
				"backendNodeId": nodeID,
			}, &result); err != nil {
				return err
			}
			var resolved struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := json.Unmarshal(result, &resolved); err != nil {
				return err
			}
			// Return a structured object so the caller can inspect (a) the match
			// strategy used and (b) the effective option value that was applied.
			js := `function(v) {
				var opts = this.options ? Array.from(this.options) : null;
				if (!opts) {
					// Not a <select>; preserve legacy behavior.
					this.value = v;
					this.dispatchEvent(new Event('input', {bubbles: true}));
					this.dispatchEvent(new Event('change', {bubbles: true}));
					return { ok: true, matchedBy: "legacy", value: v };
				}
				var target = String(v);
				var trimmed = target.trim();
				var lower = trimmed.toLowerCase();
				var match = null, matchedBy = "";
				// 1. Exact value attribute (the canonical form).
				for (var i = 0; i < opts.length; i++) {
					if (opts[i].value === target) { match = opts[i]; matchedBy = "value"; break; }
				}
				// 2. Exact (trimmed) visible text.
				if (!match) {
					for (var i = 0; i < opts.length; i++) {
						if ((opts[i].text || "").trim() === trimmed) { match = opts[i]; matchedBy = "text"; break; }
					}
				}
				// 3. Case-insensitive (trimmed) visible text.
				if (!match) {
					for (var i = 0; i < opts.length; i++) {
						if ((opts[i].text || "").trim().toLowerCase() === lower) { match = opts[i]; matchedBy = "text-ci"; break; }
					}
				}
				// 4. Case-insensitive substring of visible text (last resort).
				//    First match wins, so the user should prefer exact forms
				//    when options share a common prefix. This rescues natural
				//    shorthand like "Dark" matching the option "Dark Mode".
				if (!match && lower.length > 0) {
					for (var i = 0; i < opts.length; i++) {
						if ((opts[i].text || "").trim().toLowerCase().indexOf(lower) !== -1) { match = opts[i]; matchedBy = "text-contains"; break; }
					}
				}
				if (!match) {
					return {
						ok: false,
						error: "no option matched " + JSON.stringify(v) + " by value or visible text",
						available: opts.map(function (o) { return { value: o.value, text: (o.text || "").trim() }; })
					};
				}
				this.value = match.value;
				this.dispatchEvent(new Event('input', {bubbles: true}));
				this.dispatchEvent(new Event('change', {bubbles: true}));
				return { ok: true, matchedBy: matchedBy, value: match.value, text: (match.text || "").trim() };
			}`
			var callResult json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
				"functionDeclaration": js,
				"objectId":            resolved.Object.ObjectID,
				"arguments":           []map[string]any{{"value": value}},
				"returnByValue":       true,
			}, &callResult); err != nil {
				return err
			}
			var cr struct {
				Result struct {
					Value json.RawMessage `json:"value"`
				} `json:"result"`
			}
			if err := json.Unmarshal(callResult, &cr); err != nil {
				return err
			}
			var outcome struct {
				OK        bool   `json:"ok"`
				Error     string `json:"error"`
				MatchedBy string `json:"matchedBy"`
				Available []struct {
					Value string `json:"value"`
					Text  string `json:"text"`
				} `json:"available"`
			}
			if err := json.Unmarshal(cr.Result.Value, &outcome); err != nil {
				// Callers that use this helper on non-JSON-serializable
				// outcomes (shouldn't happen given the JS above) get the raw
				// error surfaced instead of a silent success.
				return fmt.Errorf("parse select result: %w", err)
			}
			if !outcome.OK {
				return fmt.Errorf("%s", outcome.Error)
			}
			return nil
		}),
	)
}

// ReadInputValue reads back the effective value of an input element. For React
// controlled inputs it checks the fiber's memoizedProps.value (which reflects
// React state) rather than the DOM value, since the DOM value can be stale.
// Returns the effective value the framework considers current.
func ReadInputValue(ctx context.Context, nodeID int64) (string, error) {
	var value string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var result json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &result); err != nil {
			return err
		}
		var resolved struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		if err := json.Unmarshal(result, &resolved); err != nil {
			return err
		}
		js := `function() {
			var el = this;
			var fiberKey = Object.keys(el).find(function(k) {
				return k.startsWith('__reactFiber$') || k.startsWith('__reactInternalInstance$');
			});
			if (fiberKey) {
				var fiber = el[fiberKey];
				var props = fiber && fiber.memoizedProps;
				if (props && 'value' in props) {
					return props.value || "";
				}
			}
			return el.value || "";
		}`
		var callResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": js,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult); err != nil {
			return err
		}
		var cr struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(callResult, &cr); err != nil {
			return err
		}
		value = cr.Result.Value
		return nil
	}))
	return value, err
}

func ScrollByNodeID(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": nodeID}, nil)
	}))
}
