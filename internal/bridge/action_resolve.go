package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/selector"
)

// ResolveXPathToNodeID resolves an XPath expression to a backend node ID
// using CDP's DOM.performSearch + DOM.getSearchResults.
func ResolveXPathToNodeID(ctx context.Context, xpath string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Use DOM.getDocument first to ensure the DOM is available.
		var docResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getDocument", map[string]any{"depth": 0}, &docResult); err != nil {
			return fmt.Errorf("get document: %w", err)
		}

		// Perform XPath search.
		var searchResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.performSearch", map[string]any{
			"query": xpath,
		}, &searchResult); err != nil {
			return fmt.Errorf("xpath search: %w", err)
		}

		var sr struct {
			SearchID    string `json:"searchId"`
			ResultCount int    `json:"resultCount"`
		}
		if err := json.Unmarshal(searchResult, &sr); err != nil {
			return err
		}
		if sr.ResultCount == 0 {
			return fmt.Errorf("xpath %q: no elements found", xpath)
		}

		// Get the first result.
		var getResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getSearchResults", map[string]any{
			"searchId":  sr.SearchID,
			"fromIndex": 0,
			"toIndex":   1,
		}, &getResult); err != nil {
			return fmt.Errorf("get search results: %w", err)
		}

		var gr struct {
			NodeIDs []int64 `json:"nodeIds"`
		}
		if err := json.Unmarshal(getResult, &gr); err != nil {
			return err
		}
		if len(gr.NodeIDs) == 0 {
			return fmt.Errorf("xpath %q: no node IDs returned", xpath)
		}

		// Convert DOM NodeID → BackendNodeID.
		var descResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": gr.NodeIDs[0],
		}, &descResult); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}

		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := json.Unmarshal(descResult, &desc); err != nil {
			return err
		}
		backendNodeID = desc.Node.BackendNodeID

		// Clean up the search.
		_ = chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.discardSearchResults", map[string]any{
			"searchId": sr.SearchID,
		}, nil)

		return nil
	}))
	return backendNodeID, err
}

// ResolveTextToNodeID finds the first element whose visible text content
// contains the given string and returns its backend node ID.
func ResolveTextToNodeID(ctx context.Context, text string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var docResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    "document",
			"returnByValue": false,
		}, &docResult); err != nil {
			return fmt.Errorf("resolve document: %w", err)
		}

		var doc struct {
			Result struct {
				ObjectID string `json:"objectId"`
			} `json:"result"`
		}
		if err := json.Unmarshal(docResult, &doc); err != nil {
			return err
		}
		if doc.Result.ObjectID == "" {
			return fmt.Errorf("document object not found")
		}

		const findTextFn = `function(needle) {
			const root = this.body || this.documentElement;
			if (!root) {
				return null;
			}

			const normalize = (value) => String(value || "")
				.toLowerCase()
				.replace(/\s+/g, " ")
				.trim();

			const semanticWeight = (el) => {
				const tag = (el.tagName || "").toLowerCase();
				if (tag === "button" || tag === "a" || tag === "input") {
					return 0.25;
				}
				const role = normalize(el.getAttribute && el.getAttribute("role"));
				if (role === "button" || role === "link" || role === "textbox") {
					return 0.2;
				}
				return 0;
			};

			const needleNorm = normalize(needle);
			if (!needleNorm) {
				return null;
			}

			const elements = root.querySelectorAll("*");
			let best = null;
			let bestScore = 0;

			for (const el of elements) {
				const rawText = el.innerText || el.textContent || "";
				const textNorm = normalize(rawText);
				if (!textNorm) {
					continue;
				}

				let childContains = false;
				for (const child of el.children) {
					const childNorm = normalize(child.innerText || child.textContent || "");
					if (childNorm && childNorm.includes(needleNorm)) {
						childContains = true;
						break;
					}
				}

				if (textNorm.includes(needleNorm) && !childContains) {
					return el;
				}

				// Fuzzy fallback: token-overlap score with semantic weighting.
				const tokens = needleNorm.split(" ").filter(Boolean);
				if (tokens.length === 0) {
					continue;
				}
				let hits = 0;
				for (const token of tokens) {
					if (textNorm.includes(token)) {
						hits++;
					}
				}
				let score = hits / tokens.length;
				score += semanticWeight(el);
				if (score > bestScore) {
					bestScore = score;
					best = el;
				}
			}

			if (best && bestScore >= 0.7) {
				return best;
			}
			return null;
		}`

		var callResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": findTextFn,
			"objectId":            doc.Result.ObjectID,
			"arguments":           []map[string]any{{"value": text}},
			"returnByValue":       false,
		}, &callResult); err != nil {
			return fmt.Errorf("find text node: %w", err)
		}

		var call struct {
			Result struct {
				ObjectID string `json:"objectId"`
				Subtype  string `json:"subtype"`
			} `json:"result"`
		}
		if err := json.Unmarshal(callResult, &call); err != nil {
			return err
		}
		if call.Result.ObjectID == "" || call.Result.Subtype == "null" {
			return fmt.Errorf("text %q not found", text)
		}

		var nodeResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.requestNode", map[string]any{
			"objectId": call.Result.ObjectID,
		}, &nodeResult); err != nil {
			return fmt.Errorf("request node: %w", err)
		}

		var node struct {
			NodeID int64 `json:"nodeId"`
		}
		if err := json.Unmarshal(nodeResult, &node); err != nil {
			return err
		}
		if node.NodeID == 0 {
			return fmt.Errorf("text %q resolved to an invalid node", text)
		}

		var descResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": node.NodeID,
		}, &descResult); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}

		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := json.Unmarshal(descResult, &desc); err != nil {
			return err
		}
		if desc.Node.BackendNodeID == 0 {
			return fmt.Errorf("text %q resolved to an invalid backend node", text)
		}

		backendNodeID = desc.Node.BackendNodeID
		return nil
	}))
	return backendNodeID, err
}

// ResolveCSSToNodeID resolves a CSS selector to a backend node ID.
func ResolveCSSToNodeID(ctx context.Context, css string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var docResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getDocument", map[string]any{"depth": 0}, &docResult); err != nil {
			return fmt.Errorf("get document: %w", err)
		}
		var doc struct {
			Root struct {
				NodeID int64 `json:"nodeId"`
			} `json:"root"`
		}
		if err := json.Unmarshal(docResult, &doc); err != nil {
			return err
		}

		var qResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.querySelector", map[string]any{
			"nodeId":   doc.Root.NodeID,
			"selector": css,
		}, &qResult); err != nil {
			return fmt.Errorf("querySelector: %w", err)
		}
		var qr struct {
			NodeID int64 `json:"nodeId"`
		}
		if err := json.Unmarshal(qResult, &qr); err != nil {
			return err
		}
		if qr.NodeID == 0 {
			return fmt.Errorf("css %q: no element found", css)
		}

		var descResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": qr.NodeID,
		}, &descResult); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}
		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := json.Unmarshal(descResult, &desc); err != nil {
			return err
		}
		backendNodeID = desc.Node.BackendNodeID
		return nil
	}))
	return backendNodeID, err
}

// ResolveUnifiedSelector resolves a parsed selector to a backend node ID.
// For ref selectors, the refCache is consulted. For CSS, XPath, and text
// selectors, CDP is used directly. Semantic selectors are not resolved here
// (they require the semantic matcher at a higher layer).
func ResolveUnifiedSelector(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	switch sel.Kind {
	case selector.KindRef:
		if refCache != nil {
			if nid, ok := refCache.Refs[sel.Value]; ok {
				return nid, nil
			}
		}
		return 0, fmt.Errorf("ref %s not found in snapshot cache", sel.Value)

	case selector.KindCSS:
		return ResolveCSSToNodeID(ctx, sel.Value)

	case selector.KindXPath:
		return ResolveXPathToNodeID(ctx, sel.Value)

	case selector.KindText:
		return ResolveTextToNodeID(ctx, sel.Value)

	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")

	default:
		return 0, fmt.Errorf("unknown selector kind: %q", sel.Kind)
	}
}
