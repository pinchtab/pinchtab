package cdpops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// SelectOption is one <option> of a <select>, as its value attribute and its
// trimmed visible text.
type SelectOption struct {
	Value string `json:"value"`
	Text  string `json:"text"`
}

// NoOptionMatchError reports a <select> whose requested value matched no option.
// It carries every option so the caller can pick one without re-inspecting, and
// so a handler can classify the miss as a client error rather than a server fault.
type NoOptionMatchError struct {
	Value     string
	Available []SelectOption
}

func (e *NoOptionMatchError) Error() string {
	return fmt.Sprintf("no option matched %q by value or visible text", e.Value)
}

// NotFillableError reports a fill target that holds no value of its own, such as
// one segment of a date or time input. Host names the input it belongs to, when
// the target sits in that input's shadow tree.
type NotFillableError struct {
	Tag  string `json:"tag"`
	Role string `json:"role"`
	Host *struct {
		Tag  string `json:"tag"`
		Type string `json:"type"`
	} `json:"host"`
}

func (e *NotFillableError) Error() string {
	target := "<" + e.Tag
	if e.Role != "" {
		target += fmt.Sprintf(" role=%q", e.Role)
	}
	target += ">"
	if e.Host == nil {
		return fmt.Sprintf("fill target %s is not an input, textarea, select or contenteditable element", target)
	}
	return fmt.Sprintf(
		"fill target %s is a part of <%s type=%q>, not a field; fill the host input by selector with the whole value in ISO format (e.g. 12:30, 2026-10-02)",
		target, e.Host.Tag, e.Host.Type,
	)
}

func FillByNodeID(ctx context.Context, nodeID int64, value string) error {
	objectID, err := ResolveNodeObjectID(ctx, nodeID)
	if err != nil {
		return err
	}
	if err := focusNode(ctx, nodeID); err != nil {
		return err
	}
	// The native setter of the concrete element type bypasses framework-patched
	// setters (e.g. React's value tracker); the input setter throws on a textarea.
	js := `function(v) {
				var proto = null;
				if (this instanceof window.HTMLTextAreaElement) {
					proto = window.HTMLTextAreaElement.prototype;
				} else if (this instanceof window.HTMLInputElement) {
					proto = window.HTMLInputElement.prototype;
				} else if (this instanceof window.HTMLSelectElement) {
					proto = window.HTMLSelectElement.prototype;
				}
				if (proto) {
					Object.getOwnPropertyDescriptor(proto, 'value').set.call(this, v);
				} else if (this.isContentEditable) {
					this.textContent = v;
				} else {
					var root = this.getRootNode && this.getRootNode();
					var host = root && root.host;
					return {
						tag: (this.localName || this.nodeName || '').toLowerCase(),
						role: (this.getAttribute && this.getAttribute('role')) || '',
						host: host ? {tag: host.localName, type: host.getAttribute('type') || ''} : null
					};
				}
				this.dispatchEvent(new Event('input', {bubbles: true}));
				this.dispatchEvent(new Event('change', {bubbles: true}));
				return null;
			}`
	var refusal *NotFillableError
	if err := cdptk.CallFunctionOn(ctx, objectID, js, []map[string]any{{"value": value}}, &refusal); err != nil {
		return err
	}
	if refusal != nil {
		return refusal
	}
	return nil
}

type RejectedValueError struct {
	InputType string
	Format    string
}

func (e *RejectedValueError) Error() string {
	return fmt.Sprintf(
		"<input type=%q> rejected the value and is now empty; fill it in ISO format %s",
		e.InputType, e.Format,
	)
}

var isoFormatByInputType = map[string]string{
	"date":           "YYYY-MM-DD",
	"time":           "HH:MM",
	"datetime-local": "YYYY-MM-DDTHH:MM",
	"month":          "YYYY-MM",
	"week":           "YYYY-Www",
}

func RejectedISOValue(ctx context.Context, nodeID int64) (*RejectedValueError, error) {
	js := `function() {
		if (!(this instanceof window.HTMLInputElement)) return "";
		var proto = window.HTMLInputElement.prototype;
		if (Object.getOwnPropertyDescriptor(proto, 'value').get.call(this) !== "") return "";
		return Object.getOwnPropertyDescriptor(proto, 'type').get.call(this);
	}`
	var emptyInputType string
	if err := CallFunctionOnNode(ctx, nodeID, js, nil, &emptyInputType); err != nil {
		return nil, err
	}
	format, ok := isoFormatByInputType[emptyInputType]
	if !ok {
		return nil, nil
	}
	return &RejectedValueError{InputType: emptyInputType, Format: format}, nil
}

func focusNode(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
	}))
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
	objectID, err := ResolveNodeObjectID(ctx, nodeID)
	if err != nil {
		return err
	}
	if err := focusNode(ctx, nodeID); err != nil {
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
	var raw json.RawMessage
	if err := cdptk.CallFunctionOn(ctx, objectID, js, []map[string]any{{"value": value}}, &raw); err != nil {
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
	if err := json.Unmarshal(raw, &outcome); err != nil {
		// Callers that use this helper on non-JSON-serializable
		// outcomes (shouldn't happen given the JS above) get the raw
		// error surfaced instead of a silent success.
		return fmt.Errorf("parse select result: %w", err)
	}
	if !outcome.OK {
		available := make([]SelectOption, len(outcome.Available))
		for i, o := range outcome.Available {
			available[i] = SelectOption{Value: o.Value, Text: o.Text}
		}
		return &NoOptionMatchError{Value: value, Available: available}
	}
	return nil
}

// ReadInputValue reads back the effective value of an input element. For React
// controlled inputs it checks the fiber's memoizedProps.value (which reflects
// React state) rather than the DOM value, since the DOM value can be stale.
// Returns the effective value the framework considers current. Only the native
// value getter is read, so an expando a fill left behind cannot echo back.
func ReadInputValue(ctx context.Context, nodeID int64) (string, error) {
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
		var fieldType = [window.HTMLInputElement, window.HTMLTextAreaElement, window.HTMLSelectElement]
			.find(function(type) { return el instanceof type; });
		if (fieldType) {
			return Object.getOwnPropertyDescriptor(fieldType.prototype, 'value').get.call(el) || "";
		}
		return el.isContentEditable ? el.textContent || "" : "";
	}`
	var value string
	err := CallFunctionOnNode(ctx, nodeID, js, nil, &value)
	return value, err
}
