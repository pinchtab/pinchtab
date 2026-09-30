package bridge

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

type nodeDOMMetadata struct {
	Tag          string `json:"tag"`
	Label        string `json:"label"`
	Placeholder  string `json:"placeholder"`
	Alt          string `json:"alt"`
	Title        string `json:"title"`
	TestID       string `json:"testid"`
	Text         string `json:"text"`
	InputType    string `json:"inputType"`
	Autocomplete string `json:"autocomplete"`
	// HasValue is the DOM's own emptiness answer (!!el.value). The value itself
	// never crosses this boundary, and the accessibility tree cannot answer it —
	// browsers routinely suppress AXValue for password inputs, so gating on the
	// node's existing value would render a filled password as empty.
	HasValue bool `json:"hasValue"`
}

// EnrichA11yNodesWithDOMMetadata adds DOM-backed descriptor fields used by the
// semantic structured locator matcher. It is best-effort: per-node CDP failures
// are skipped so snapshots still work on pages with detached or remote nodes.
func EnrichA11yNodesWithDOMMetadata(ctx context.Context, nodes []A11yNode) error {
	if len(nodes) == 0 {
		return nil
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		c := chromedp.FromContext(ctx)
		if c == nil || c.Target == nil {
			return nil
		}
		for i := range nodes {
			if nodes[i].NodeID == 0 || hasNodeDOMMetadata(nodes[i]) {
				continue
			}
			meta, ok := resolveNodeDOMMetadata(ctx, nodes[i].NodeID)
			if !ok {
				continue
			}
			applyNodeDOMMetadata(&nodes[i], meta)
		}
		return nil
	}))
}

func hasNodeDOMMetadata(node A11yNode) bool {
	return node.Tag != "" ||
		node.Label != "" ||
		node.Placeholder != "" ||
		node.Alt != "" ||
		node.Title != "" ||
		node.TestID != "" ||
		node.Text != ""
}

func resolveNodeDOMMetadata(ctx context.Context, backendNodeID int64) (nodeDOMMetadata, bool) {
	objectID, err := bridgecdpops.ResolveNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return nodeDOMMetadata{}, false
	}
	defer func() {
		_ = chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.releaseObject", map[string]any{
			"objectId": objectID,
		}, nil)
	}()

	var callResult json.RawMessage
	if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
		"functionDeclaration": domMetadataFn,
		"objectId":            objectID,
		"returnByValue":       true,
	}, &callResult); err != nil {
		return nodeDOMMetadata{}, false
	}

	var call struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(callResult, &call); err != nil || len(call.ExceptionDetails) > 0 || len(call.Result.Value) == 0 {
		return nodeDOMMetadata{}, false
	}

	var meta nodeDOMMetadata
	if err := json.Unmarshal(call.Result.Value, &meta); err != nil {
		return nodeDOMMetadata{}, false
	}
	return meta, true
}

func applyNodeDOMMetadata(node *A11yNode, meta nodeDOMMetadata) {
	node.Tag = strings.TrimSpace(meta.Tag)
	node.Label = strings.TrimSpace(meta.Label)
	node.Placeholder = strings.TrimSpace(meta.Placeholder)
	node.Alt = strings.TrimSpace(meta.Alt)
	node.Title = strings.TrimSpace(meta.Title)
	node.TestID = strings.TrimSpace(meta.TestID)
	node.Text = strings.TrimSpace(meta.Text)
	if !isSensitiveField(meta) {
		return
	}
	// This is the only site allowed to clear a mask, and only on the DOM's
	// positive "no value" answer: an empty field must read as empty, a filled one
	// must never show its content.
	if meta.HasValue {
		node.Value = bridgeobserve.MaskedValue
		return
	}
	node.Value = ""
}

// isSensitiveField is the union of the two password signals: the input type and
// the autocomplete token. Either one alone marks the field sensitive.
func isSensitiveField(meta nodeDOMMetadata) bool {
	return strings.EqualFold(strings.TrimSpace(meta.InputType), "password") ||
		bridgeobserve.IsSensitiveAutocomplete(meta.Autocomplete)
}

const domMetadataFn = `function() {
	const el = this;
	if (!el || el.nodeType !== 1) return {};
	const doc = el.ownerDocument || document;
	const normalize = (value) => String(value || "").replace(/\s+/g, " ").trim();
	const attr = (name) => el.getAttribute ? el.getAttribute(name) || "" : "";
	const unique = (items) => {
		const seen = new Set();
		const out = [];
		for (const item of items) {
			const value = normalize(item);
			if (!value || seen.has(value.toLowerCase())) continue;
			seen.add(value.toLowerCase());
			out.push(value);
		}
		return out;
	};
	const labelledByText = () => {
		const labelledBy = attr("aria-labelledby");
		if (!labelledBy) return "";
		return labelledBy.split(/\s+/)
			.map((id) => doc.getElementById && doc.getElementById(id))
			.filter(Boolean)
			.map((node) => node.textContent || "")
			.join(" ");
	};
	const labelText = () => {
		const parts = [];
		if (el.labels) {
			for (const label of el.labels) parts.push(label.textContent || "");
		}
		const id = el.id;
		if (id && doc.querySelectorAll) {
			for (const label of doc.querySelectorAll("label")) {
				if (label.getAttribute("for") === id) parts.push(label.textContent || "");
			}
		}
		return parts.join(" ");
	};
	const testID = () => {
		for (const name of ["data-testid", "data-test-id", "data-test", "data-qa", "testid"]) {
			const value = attr(name);
			if (normalize(value)) return value;
		}
		return "";
	};
	const text = normalize(typeof el.innerText === "string" ? el.innerText : el.textContent || "");
	return {
		tag: String(el.tagName || "").toLowerCase(),
		label: unique([labelledByText(), labelText(), attr("aria-label")]).join(" "),
		placeholder: attr("placeholder"),
		alt: attr("alt"),
		title: attr("title"),
		testid: testID(),
		text: text.length > 500 ? text.slice(0, 500) : text,
		inputType: (el.tagName && el.tagName.toLowerCase() === "input") ? (el.type || "") : "",
		autocomplete: attr("autocomplete"),
		hasValue: !!el.value
	};
}`
