package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/selector"
)

type FrameElementMeta struct {
	TagName string `json:"tagName"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Title   string `json:"title,omitempty"`
	Src     string `json:"src,omitempty"`
}

// FrameExecutionContextID returns a Runtime.executionContextId that
// evaluates in the given frame's document. Safe to call from other packages
// that need to scope `Runtime.evaluate` / `Runtime.callFunctionOn` to a
// frame (for example, the /text handler when a frame scope is active).
// Passes frameID == "" through as a no-op (returns 0, nil) so callers can
// fall back to the default top-level context without branching.
func FrameExecutionContextID(ctx context.Context, frameID string) (int64, error) {
	return frameExecutionContextID(ctx, frameID)
}

func frameExecutionContextID(ctx context.Context, frameID string) (int64, error) {
	if frameID == "" {
		return 0, nil
	}

	var worldResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.createIsolatedWorld", map[string]any{
			"frameId":   frameID,
			"worldName": "pinchtab-frame-scope",
		}, &worldResult)
	}))
	if err != nil {
		return 0, fmt.Errorf("create isolated world for frame %q: %w", frameID, err)
	}

	var resp struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := json.Unmarshal(worldResult, &resp); err != nil {
		return 0, err
	}
	if resp.ExecutionContextID == 0 {
		return 0, fmt.Errorf("frame %q has no execution context", frameID)
	}
	return resp.ExecutionContextID, nil
}

func frameDocumentObjectID(ctx context.Context, frameID string) (string, error) {
	params := map[string]any{
		"expression":    "document",
		"returnByValue": false,
	}
	if frameID != "" {
		execID, err := frameExecutionContextID(ctx, frameID)
		if err != nil {
			return "", err
		}
		params["contextId"] = execID
	}

	var docResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", params, &docResult)
	}))
	if err != nil {
		return "", fmt.Errorf("resolve document: %w", err)
	}

	var doc struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if err := json.Unmarshal(docResult, &doc); err != nil {
		return "", err
	}
	if doc.Result.ObjectID == "" {
		return "", fmt.Errorf("document object not found")
	}
	return doc.Result.ObjectID, nil
}

func backendNodeIDFromObjectID(ctx context.Context, objectID string) (int64, error) {
	var nodeResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.requestNode", map[string]any{
			"objectId": objectID,
		}, &nodeResult)
	}))
	if err != nil {
		return 0, fmt.Errorf("request node: %w", err)
	}

	var node struct {
		NodeID int64 `json:"nodeId"`
	}
	if err := json.Unmarshal(nodeResult, &node); err != nil {
		return 0, err
	}
	if node.NodeID == 0 {
		return 0, fmt.Errorf("resolved to an invalid node")
	}

	var descResult json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": node.NodeID,
		}, &descResult)
	}))
	if err != nil {
		return 0, fmt.Errorf("describe node: %w", err)
	}

	var desc struct {
		Node struct {
			BackendNodeID int64 `json:"backendNodeId"`
		} `json:"node"`
	}
	if err := json.Unmarshal(descResult, &desc); err != nil {
		return 0, err
	}
	if desc.Node.BackendNodeID == 0 {
		return 0, fmt.Errorf("resolved to an invalid backend node")
	}
	return desc.Node.BackendNodeID, nil
}

func resolveNodeInFrame(ctx context.Context, frameID, functionDeclaration string, args []map[string]any) (int64, error) {
	docObjectID, err := frameDocumentObjectID(ctx, frameID)
	if err != nil {
		return 0, err
	}

	var callResult json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": functionDeclaration,
			"objectId":            docObjectID,
			"arguments":           args,
			"returnByValue":       false,
		}, &callResult)
	}))
	if err != nil {
		return 0, err
	}

	var call struct {
		Result struct {
			Type     string `json:"type"`
			Subtype  string `json:"subtype"`
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &call); err != nil {
		return 0, err
	}
	if call.Result.ObjectID == "" || call.Result.Subtype == "null" || call.Result.Type == "undefined" {
		return 0, fmt.Errorf("no element found")
	}

	return backendNodeIDFromObjectID(ctx, call.Result.ObjectID)
}

func resolveElementMetaInFrame(ctx context.Context, frameID, functionDeclaration string, args []map[string]any) (FrameElementMeta, error) {
	docObjectID, err := frameDocumentObjectID(ctx, frameID)
	if err != nil {
		return FrameElementMeta{}, err
	}

	var callResult json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": functionDeclaration,
			"objectId":            docObjectID,
			"arguments":           args,
			"returnByValue":       true,
		}, &callResult)
	}))
	if err != nil {
		return FrameElementMeta{}, err
	}

	var call struct {
		Result struct {
			Type    string          `json:"type"`
			Subtype string          `json:"subtype"`
			Value   json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &call); err != nil {
		return FrameElementMeta{}, err
	}
	if call.Result.Subtype == "null" || call.Result.Type == "undefined" || len(call.Result.Value) == 0 || string(call.Result.Value) == "null" {
		return FrameElementMeta{}, fmt.Errorf("no element found")
	}

	var meta FrameElementMeta
	if err := json.Unmarshal(call.Result.Value, &meta); err != nil {
		return FrameElementMeta{}, err
	}
	meta.TagName = strings.ToLower(meta.TagName)
	return meta, nil
}

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
	return ResolveTextToNodeIDInFrame(ctx, "", text)
}

func ResolveTextToNodeIDInFrame(ctx context.Context, frameID, text string) (int64, error) {
	var backendNodeID int64
	// Implementation notes:
	//   - Use `textContent` (not `innerText`) for the bulk scan. `innerText`
	//     forces a synchronous layout pass per-element and is O(N^2) on large
	//     pages; `textContent` is O(N). This fixes the intermittent
	//     "context deadline exceeded" failures on dynamic/large fixtures.
	//   - Exact-match pass first (single linear sweep). Fuzzy fallback is
	//     only evaluated when no exact hit fires — most real lookups are
	//     covered by the exact pass and cost nothing extra.
	//   - "Leaf-most match wins": we keep the smallest element (by
	//     descendant count) whose text contains the needle, so a button
	//     that reads "Sign In" is preferred over its ancestor <body> which
	//     technically also contains the string.
	const findTextFn = `function(needle) {
			const root = this.body || this.documentElement;
			if (!root) return null;

			const normalize = (value) => String(value || "")
				.toLowerCase()
				.replace(/\s+/g, " ")
				.trim();
			const semanticWeight = (el) => {
				const tag = (el.tagName || "").toLowerCase();
				if (tag === "button" || tag === "a" || tag === "input") return 0.25;
				const role = normalize(el.getAttribute && el.getAttribute("role"));
				if (role === "button" || role === "link" || role === "textbox") return 0.2;
				return 0;
			};

			const needleNorm = normalize(needle);
			if (!needleNorm) return null;

			const elements = root.querySelectorAll("*");

			// Exact-match pass: pick the leaf-most element whose textContent
			// contains the needle. textContent is cheap (no layout), so we
			// can afford to visit every node.
			let exactBest = null;
			let exactBestSize = Infinity;
			for (const el of elements) {
				const tc = normalize(el.textContent || "");
				if (!tc || !tc.includes(needleNorm)) continue;
				// "Leaf-most" = fewest descendants. Smaller subtree == more
				// specific match. Ties broken by semantic weight.
				const size = el.getElementsByTagName("*").length;
				if (size < exactBestSize ||
					(size === exactBestSize && exactBest && semanticWeight(el) > semanticWeight(exactBest))) {
					exactBest = el;
					exactBestSize = size;
				}
			}
			if (exactBest) return exactBest;

			// Fuzzy fallback: token-overlap score with semantic weighting.
			// Only runs if exact-match missed.
			const tokens = needleNorm.split(" ").filter(Boolean);
			if (tokens.length === 0) return null;

			let best = null;
			let bestScore = 0;
			for (const el of elements) {
				const tc = normalize(el.textContent || "");
				if (!tc) continue;
				let hits = 0;
				for (const token of tokens) {
					if (tc.includes(token)) hits++;
				}
				let score = hits / tokens.length + semanticWeight(el);
				if (score > bestScore) {
					bestScore = score;
					best = el;
				}
			}
			return (best && bestScore >= 0.7) ? best : null;
		}`

	// Bound the lookup with its own short deadline so a slow resolution
	// can't eat the entire action timeout. Callers can still pass a longer
	// outer deadline if they really want to wait — this just caps how long
	// we'll spend before giving up with "text not found".
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err := chromedp.Run(lookupCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		nid, err := resolveNodeInFrame(ctx, frameID, findTextFn, []map[string]any{{"value": text}})
		if err != nil {
			// If the parent context is still alive, this was a real
			// "not found" rather than a timeout — surface it clearly.
			if lookupCtx.Err() != nil && ctx.Err() == nil {
				return fmt.Errorf("text %q lookup timed out after 3s (page may be large or unresponsive): %w", text, err)
			}
			return fmt.Errorf("text %q not found: %w", text, err)
		}
		backendNodeID = nid
		return nil
	}))
	return backendNodeID, err
}

// ResolveCSSToNodeID resolves a CSS selector to a backend node ID.
func ResolveCSSToNodeID(ctx context.Context, css string) (int64, error) {
	return ResolveCSSToNodeIDInFrame(ctx, "", css)
}

func ResolveCSSToNodeIDInFrame(ctx context.Context, frameID, css string) (int64, error) {
	if frameID == "" {
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

	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		nid, err := resolveNodeInFrame(ctx, frameID, `function(selector) { return this.querySelector(selector); }`, []map[string]any{{"value": css}})
		if err != nil {
			return fmt.Errorf("css %q: %w", css, err)
		}
		backendNodeID = nid
		return nil
	}))
	return backendNodeID, err
}

func ResolveXPathToNodeIDInFrame(ctx context.Context, frameID, xpath string) (int64, error) {
	if frameID == "" {
		return ResolveXPathToNodeID(ctx, xpath)
	}

	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		nid, err := resolveNodeInFrame(ctx, frameID, `function(xpath) {
			return this.evaluate(xpath, this, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		}`, []map[string]any{{"value": xpath}})
		if err != nil {
			return fmt.Errorf("xpath %q: %w", xpath, err)
		}
		backendNodeID = nid
		return nil
	}))
	return backendNodeID, err
}

func ResolveFrameElementMetaInFrame(ctx context.Context, sel selector.Selector, frameID string) (FrameElementMeta, error) {
	switch sel.Kind {
	case selector.KindCSS:
		return resolveElementMetaInFrame(ctx, frameID, `function(selector) {
			const el = this.querySelector(selector);
			if (!el) {
				return null;
			}
			return {
				tagName: (el.tagName || "").toLowerCase(),
				id: el.id || "",
				name: el.getAttribute("name") || "",
				title: el.getAttribute("title") || "",
				src: el.src || ""
			};
		}`, []map[string]any{{"value": sel.Value}})
	case selector.KindXPath:
		return resolveElementMetaInFrame(ctx, frameID, `function(xpath) {
			const el = this.evaluate(xpath, this, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
			if (!el) {
				return null;
			}
			return {
				tagName: (el.tagName || "").toLowerCase(),
				id: el.id || "",
				name: el.getAttribute && el.getAttribute("name") || "",
				title: el.getAttribute && el.getAttribute("title") || "",
				src: el.src || ""
			};
		}`, []map[string]any{{"value": sel.Value}})
	default:
		return FrameElementMeta{}, fmt.Errorf("frame element metadata requires css or xpath selector")
	}
}

// ResolveUnifiedSelectorInFrame resolves a parsed selector to a backend node ID.
// Ref selectors still use the ref cache directly; non-ref selectors honor the
// provided frame scope.
func ResolveUnifiedSelectorInFrame(ctx context.Context, sel selector.Selector, refCache *RefCache, frameID string) (int64, error) {
	switch sel.Kind {
	case selector.KindRef:
		if refCache != nil {
			if target, ok := refCache.Lookup(sel.Value); ok {
				return target.BackendNodeID, nil
			}
		}
		return 0, fmt.Errorf("ref %s not found in snapshot cache", sel.Value)

	case selector.KindCSS:
		return ResolveCSSToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindXPath:
		return ResolveXPathToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindText:
		return ResolveTextToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")

	default:
		return 0, fmt.Errorf("unknown selector kind: %q", sel.Kind)
	}
}

// ResolveUnifiedSelector resolves a parsed selector to a backend node ID.
// For ref selectors, the refCache is consulted. For CSS, XPath, and text
// selectors, CDP is used directly. Semantic selectors are not resolved here
// (they require the semantic matcher at a higher layer).
func ResolveUnifiedSelector(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	return ResolveUnifiedSelectorInFrame(ctx, sel, refCache, "")
}
