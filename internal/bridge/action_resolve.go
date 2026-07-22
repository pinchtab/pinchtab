package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/selector"
)

// ErrSelectorNoMatch marks a resolution failure where the selector was valid
// but matched no element (a client-side "not found"), as distinct from a
// CDP/transport fault, an unsupported selector kind, or an internal routing
// error — which must surface as 5xx, not 404. Callers classify with errors.Is.
var ErrSelectorNoMatch = errors.New("selector matched no element")

// ErrSelectorOutsideScope marks a cached ref that still exists but belongs to
// the background document rather than the active modal subtree. Callers must
// not treat this as a stale ref or invoke global semantic recovery.
var ErrSelectorOutsideScope = errors.New("selector target is outside scope")

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
	return bridgecdpops.FrameExecutionContextID(ctx, frameID)
}

func frameDocumentObjectID(ctx context.Context, frameID string) (string, error) {
	// Selector and modal discovery run in an isolated world so page script cannot
	// hide or redirect targets by replacing DOM methods in the main world.
	if frameID == "" {
		frameTree, err := FetchFrameTree(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve top frame: %w", err)
		}
		frameID = frameTree.Frame.ID
		if frameID == "" {
			return "", fmt.Errorf("resolve top frame: frame id is empty")
		}
	}
	execID, err := frameExecutionContextID(ctx, frameID)
	if err != nil {
		return "", err
	}
	if execID == 0 {
		return "", fmt.Errorf("frame %q has no isolated execution context", frameID)
	}

	params := map[string]any{
		"expression":    "document",
		"returnByValue": false,
		"contextId":     execID,
	}

	var docResult json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", params, &docResult)
	}))
	if err != nil {
		return "", fmt.Errorf("resolve document: %w", err)
	}

	var doc struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(docResult, &doc); err != nil {
		return "", err
	}
	if doc.Result.ObjectID == "" {
		if doc.ExceptionDetails != nil {
			return "", fmt.Errorf("resolve document: %s", doc.ExceptionDetails.Text)
		}
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
		return 0, fmt.Errorf("%w", ErrSelectorNoMatch)
	}

	return backendNodeIDFromObjectID(ctx, call.Result.ObjectID)
}

// TopmostModalNodeIDInFrame returns the backend node ID of the visually
// topmost visible modal owner in a frame. A missing dialog is a normal result,
// not an error. Discovery runs in an isolated world and uses browser hit
// testing rather than comparing local z-index values, which are not comparable
// across stacking contexts or the native top layer.
func TopmostModalNodeIDInFrame(ctx context.Context, frameID string) (int64, bool, error) {
	const topmostModalFn = `function() {
		const candidates = [];
		const seen = new Set();
		const composedParent = (node) => node && (node.parentNode || node.host || null);
		const composedContains = (ancestor, node) => {
			for (let cur = node; cur; cur = composedParent(cur)) if (cur === ancestor) return true;
			return false;
		};
		const visit = (root) => {
			if (!root || !root.querySelectorAll) return;
			for (const el of root.querySelectorAll('dialog, [aria-modal="true"]')) {
				if (!seen.has(el)) { seen.add(el); candidates.push(el); }
			}
			for (const el of root.querySelectorAll("*")) if (el.shadowRoot) visit(el.shadowRoot);
		};
		visit(this);
		const visible = candidates.filter((el) => {
			if (!el || !el.isConnected) return false;
			let nativeModal = false;
			try { nativeModal = el.matches(":modal"); } catch (_) {}
			if (!nativeModal && el.getAttribute("aria-modal") !== "true") return false;
			for (let cur = el; cur; cur = composedParent(cur)) {
				if (cur.nodeType !== 1) continue;
				if (cur.getAttribute("aria-hidden") === "true") return false;
				const style = cur.ownerDocument.defaultView.getComputedStyle(cur);
				if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse") return false;
				if (Number.parseFloat(style.opacity || "1") <= 0) return false;
			}
			const rect = el.getBoundingClientRect();
			return rect.width > 0 && rect.height > 0 && rect.right > 0 && rect.bottom > 0 &&
				rect.left < el.ownerDocument.defaultView.innerWidth && rect.top < el.ownerDocument.defaultView.innerHeight;
		});
		if (!visible.length) return null;

		// An inner modal owns interaction before any containing modal, regardless
		// of the outer element's local z-index.
		const leaves = visible.filter((candidate) => !visible.some((other) =>
			other !== candidate && composedContains(candidate, other)));
		const points = [];
		const pointKeys = new Set();
		const addPoint = (x, y) => {
			x = Math.max(0, Math.min(this.defaultView.innerWidth - 1, x));
			y = Math.max(0, Math.min(this.defaultView.innerHeight - 1, y));
			const key = Math.round(x) + ":" + Math.round(y);
			if (!pointKeys.has(key)) { pointKeys.add(key); points.push([x, y]); }
		};
		for (const el of leaves) {
			const r = el.getBoundingClientRect();
			const left = Math.max(0, r.left), right = Math.min(this.defaultView.innerWidth, r.right);
			const top = Math.max(0, r.top), bottom = Math.min(this.defaultView.innerHeight, r.bottom);
			addPoint((left + right) / 2, (top + bottom) / 2);
			addPoint(left + (right - left) * .2, top + (bottom - top) * .2);
			addPoint(right - (right - left) * .2, top + (bottom - top) * .2);
			addPoint(left + (right - left) * .2, bottom - (bottom - top) * .2);
			addPoint(right - (right - left) * .2, bottom - (bottom - top) * .2);
		}
		const deepHit = (x, y) => {
			let root = this, hit = null;
			while (root && root.elementFromPoint) {
				const next = root.elementFromPoint(x, y);
				if (!next || next === hit) break;
				hit = next;
				root = next.shadowRoot;
			}
			return hit;
		};
		let best = null, bestHits = 0;
		for (const candidate of leaves) {
			let hits = 0;
			for (const [x, y] of points) if (composedContains(candidate, deepHit(x, y))) hits++;
			if (hits > bestHits || (hits === bestHits && hits > 0)) {
				best = candidate;
				bestHits = hits;
			}
		}
		return bestHits > 0 ? best : null;
	}`

	nodeID, err := resolveNodeInFrame(ctx, frameID, topmostModalFn, nil)
	if errors.Is(err, ErrSelectorNoMatch) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("resolve topmost dialog: %w", err)
	}
	return nodeID, true, nil
}

// TopmostModalNodeID gives a top-document modal precedence over a caller's
// current iframe scope. This prevents a stale frame scope from interacting
// with background content underneath a page-level modal. When the top
// document has no modal, it falls back to the requested frame.
func TopmostModalNodeID(ctx context.Context, frameID string) (int64, bool, error) {
	nodeID, open, err := TopmostModalNodeIDInFrame(ctx, "")
	if err != nil || open || frameID == "" {
		return nodeID, open, err
	}
	return TopmostModalNodeIDInFrame(ctx, frameID)
}

// resolveNodeWithinBackendNode invokes functionDeclaration with the scope
// element as `this` and converts the returned DOM object to a backend node ID.
func resolveNodeWithinBackendNode(ctx context.Context, scopeBackendNodeID int64, functionDeclaration string, args []map[string]any) (int64, error) {
	var scopeResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": scopeBackendNodeID,
		}, &scopeResult)
	}))
	if err != nil {
		return 0, fmt.Errorf("resolve scope node: %w", err)
	}

	var scope struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(scopeResult, &scope); err != nil {
		return 0, err
	}
	if scope.Object.ObjectID == "" {
		return 0, fmt.Errorf("dialog scope is no longer attached")
	}

	params := map[string]any{
		"functionDeclaration": functionDeclaration,
		"objectId":            scope.Object.ObjectID,
		"returnByValue":       false,
	}
	if len(args) > 0 {
		params["arguments"] = args
	}

	var callResult json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", params, &callResult)
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
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(callResult, &call); err != nil {
		return 0, err
	}
	if call.ExceptionDetails != nil {
		return 0, fmt.Errorf("scoped selector evaluation: %s", call.ExceptionDetails.Text)
	}
	if call.Result.ObjectID == "" || call.Result.Subtype == "null" || call.Result.Type == "undefined" {
		return 0, fmt.Errorf("%w", ErrSelectorNoMatch)
	}
	return backendNodeIDFromObjectID(ctx, call.Result.ObjectID)
}

// BackendNodeWithinScope reports whether target is the scope node or one of
// its DOM descendants. It is used to reject stale/background snapshot refs
// while a modal dialog owns the interaction surface.
func BackendNodeWithinScope(ctx context.Context, scopeBackendNodeID, targetBackendNodeID int64) (bool, error) {
	if scopeBackendNodeID == 0 || targetBackendNodeID == 0 {
		return false, nil
	}

	resolve := func(ctx context.Context, backendNodeID int64) (string, error) {
		var raw json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &raw); err != nil {
			return "", err
		}
		var parsed struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", err
		}
		if parsed.Object.ObjectID == "" {
			return "", fmt.Errorf("backend node %d is no longer attached", backendNodeID)
		}
		return parsed.Object.ObjectID, nil
	}

	var contains bool
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		scopeObjectID, err := resolve(ctx, scopeBackendNodeID)
		if err != nil {
			return fmt.Errorf("resolve scope node: %w", err)
		}
		targetObjectID, err := resolve(ctx, targetBackendNodeID)
		if err != nil {
			return fmt.Errorf("resolve target node: %w", err)
		}

		var raw json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": `function(target) {
				for (let cur = target; cur; cur = cur.parentNode || cur.host || null) {
					if (cur === this) return true;
				}
				return false;
			}`,
			"objectId":      scopeObjectID,
			"arguments":     []map[string]any{{"objectId": targetObjectID}},
			"returnByValue": true,
		}, &raw); err != nil {
			return err
		}
		var parsed struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
			ExceptionDetails *struct {
				Text string `json:"text"`
			} `json:"exceptionDetails,omitempty"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return err
		}
		if parsed.ExceptionDetails != nil {
			return fmt.Errorf("scope containment check: %s", parsed.ExceptionDetails.Text)
		}
		contains = parsed.Result.Value
		return nil
	}))
	return contains, err
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

func ResolveXPathToNodeID(ctx context.Context, xpath string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Use DOM.getDocument first to ensure the DOM is available.
		var docResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getDocument", map[string]any{"depth": 0}, &docResult); err != nil {
			return fmt.Errorf("get document: %w", err)
		}

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
			return fmt.Errorf("xpath %q: %w", xpath, ErrSelectorNoMatch)
		}

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

		_ = chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.discardSearchResults", map[string]any{
			"searchId": sr.SearchID,
		}, nil)

		return nil
	}))
	return backendNodeID, err
}

// jsNormalizeHelper is the shared text-normalization snippet injected into both
// findTextFn and resolveSelectorAtFn so the two embedded JS programs can't drift
// apart. It lowercases, collapses runs of whitespace to a single space, and
// trims — identical in both call sites. Indentation here is cosmetic (JS ignores
// it); only the runtime behavior matters.
const jsNormalizeHelper = `const normalize = (value) => String(value || "")
		.toLowerCase()
		.replace(/\s+/g, " ")
		.trim();`

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

			` + jsNormalizeHelper + `
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

const resolveSelectorAtFn = `function(kind, value, index, fromEnd) {
	const root = this;
	` + jsNormalizeHelper + `
	const needle = normalize(value);
	const unique = (items) => {
		const seen = new Set();
		const out = [];
		for (const item of items) {
			if (!item || seen.has(item)) continue;
			seen.add(item);
			out.push(item);
		}
		return out;
	};
	const pick = (items) => {
		items = unique(items);
		if (!items.length) return null;
		const idx = fromEnd ? items.length - 1 : index;
		if (idx < 0 || idx >= items.length) return null;
		return items[idx];
	};
	const deepQueryAll = (selector) => {
		const out = [];
		const visit = (scope) => {
			if (!scope || !scope.querySelectorAll) return;
			if (scope.nodeType === 1 && scope.matches && scope.matches(selector)) out.push(scope);
			const elements = Array.from(scope.querySelectorAll("*"));
			out.push(...Array.from(scope.querySelectorAll(selector)));
			for (const el of elements) if (el.shadowRoot) visit(el.shadowRoot);
		};
		visit(root);
		return out;
	};
	const textCandidates = (query) => {
		const elements = deepQueryAll("*");
		const exact = [];
		for (const el of elements) {
			const text = normalize(el.textContent || "");
			if (!query || !text || !(text === query || text.includes(query))) continue;
			exact.push({ el, size: el.getElementsByTagName("*").length });
		}
		if (exact.length) {
			const minSize = Math.min(...exact.map((item) => item.size));
			return exact.filter((item) => item.size === minSize).map((item) => item.el);
		}
		const tokens = query.split(" ").filter(Boolean);
		if (!tokens.length) return [];
		const fuzzy = [];
		for (const el of elements) {
			const text = normalize(el.textContent || "");
			if (!text) continue;
			let hits = 0;
			for (const token of tokens) if (text.includes(token)) hits++;
			if (hits / tokens.length >= 0.7) {
				fuzzy.push({ el, size: el.getElementsByTagName("*").length });
			}
		}
		if (!fuzzy.length) return [];
		const minSize = Math.min(...fuzzy.map((item) => item.size));
		return fuzzy.filter((item) => item.size === minSize).map((item) => item.el);
	};

	try {
		switch (kind) {
		case "css":
			return pick(deepQueryAll(value));
		case "xpath": {
			const document = root.ownerDocument || root;
			const result = document.evaluate(value, root, null, XPathResult.ORDERED_NODE_SNAPSHOT_TYPE, null);
			const items = [];
			for (let i = 0; i < result.snapshotLength; i++) {
				const item = result.snapshotItem(i);
				if (root.nodeType === 9 || item === root || (root.contains && root.contains(item))) items.push(item);
			}
			return pick(items);
		}
		case "text":
			return pick(textCandidates(needle));
		default:
			return null;
		}
	} catch (e) {
		return null;
	}
}`

func resolveSelectorAtInFrame(ctx context.Context, frameID string, sel selector.Selector, index int, fromEnd bool) (int64, error) {
	kind := string(sel.Kind)
	switch sel.Kind {
	case selector.KindCSS, selector.KindXPath, selector.KindText:
	default:
		return 0, fmt.Errorf("%s selector cannot be used with first/last/nth", sel.Kind)
	}

	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		nid, err := resolveNodeInFrame(ctx, frameID, resolveSelectorAtFn, []map[string]any{
			{"value": kind},
			{"value": sel.Value},
			{"value": index},
			{"value": fromEnd},
		})
		if err != nil {
			return fmt.Errorf("%s %q: %w", sel.Kind, sel.Value, ErrSelectorNoMatch)
		}
		backendNodeID = nid
		return nil
	}))
	return backendNodeID, err
}

func resolveSelectorAtWithinNode(ctx context.Context, scopeBackendNodeID int64, sel selector.Selector, index int, fromEnd bool) (int64, error) {
	kind := string(sel.Kind)
	switch sel.Kind {
	case selector.KindCSS, selector.KindXPath, selector.KindText:
	default:
		return 0, fmt.Errorf("%s selector cannot be used with first/last/nth", sel.Kind)
	}

	nid, err := resolveNodeWithinBackendNode(ctx, scopeBackendNodeID, resolveSelectorAtFn, []map[string]any{
		{"value": kind},
		{"value": sel.Value},
		{"value": index},
		{"value": fromEnd},
	})
	if err != nil {
		return 0, fmt.Errorf("%s %q inside topmost dialog: %w", sel.Kind, sel.Value, err)
	}
	return nid, nil
}

func parseNthSelectorValue(value string) (int, string, error) {
	rawIndex, rawSelector, ok := strings.Cut(value, ":")
	if !ok {
		return 0, "", fmt.Errorf("nth selector requires nth:<index>:<selector>")
	}
	rawIndex = strings.TrimSpace(rawIndex)
	rawSelector = strings.TrimSpace(rawSelector)
	if rawSelector == "" {
		return 0, "", fmt.Errorf("nth selector requires a nested selector")
	}
	index, err := strconv.Atoi(rawIndex)
	if err != nil || index < 0 {
		return 0, "", fmt.Errorf("nth selector index must be a zero-based non-negative integer")
	}
	return index, rawSelector, nil
}

func resolveNestedSelectorAtInFrame(ctx context.Context, frameID string, raw string, refCache *RefCache, index int, fromEnd bool) (int64, error) {
	inner := selector.Parse(raw)
	switch inner.Kind {
	case selector.KindFirst:
		return resolveNestedSelectorAtInFrame(ctx, frameID, inner.Value, refCache, 0, false)
	case selector.KindLast:
		return resolveNestedSelectorAtInFrame(ctx, frameID, inner.Value, refCache, 0, true)
	case selector.KindNth:
		nth, nestedRaw, err := parseNthSelectorValue(inner.Value)
		if err != nil {
			return 0, err
		}
		return resolveNestedSelectorAtInFrame(ctx, frameID, nestedRaw, refCache, nth, false)
	case selector.KindRef:
		if fromEnd || index != 0 {
			return 0, fmt.Errorf("ref selector cannot be used with last/nth")
		}
		return ResolveUnifiedSelectorInFrame(ctx, inner, refCache, frameID)
	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")
	default:
		return resolveSelectorAtInFrame(ctx, frameID, inner, index, fromEnd)
	}
}

func resolveNestedSelectorWithinNode(ctx context.Context, scopeBackendNodeID int64, raw string, refCache *RefCache, index int, fromEnd bool) (int64, error) {
	inner := selector.Parse(raw)
	switch inner.Kind {
	case selector.KindFirst:
		return resolveNestedSelectorWithinNode(ctx, scopeBackendNodeID, inner.Value, refCache, 0, false)
	case selector.KindLast:
		return resolveNestedSelectorWithinNode(ctx, scopeBackendNodeID, inner.Value, refCache, 0, true)
	case selector.KindNth:
		nth, nestedRaw, err := parseNthSelectorValue(inner.Value)
		if err != nil {
			return 0, err
		}
		return resolveNestedSelectorWithinNode(ctx, scopeBackendNodeID, nestedRaw, refCache, nth, false)
	case selector.KindRef:
		if fromEnd || index != 0 {
			return 0, fmt.Errorf("ref selector cannot be used with last/nth")
		}
		return ResolveUnifiedSelectorWithinNode(ctx, inner, refCache, scopeBackendNodeID)
	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")
	default:
		return resolveSelectorAtWithinNode(ctx, scopeBackendNodeID, inner, index, fromEnd)
	}
}

func ResolveCSSToNodeID(ctx context.Context, css string) (int64, error) {
	return ResolveCSSToNodeIDInFrame(ctx, "", css)
}

func ResolveCSSToNodeIDInFrame(ctx context.Context, frameID, css string) (int64, error) {
	// Resolve through the deep selector walker (resolveSelectorAtFn → deepQueryAll)
	// so a CSS selector matches the first element even when it is nested in an open
	// shadow root, not just the light DOM (issue #591). For light-DOM pages this
	// returns the same first match as document.querySelector, and it works for both
	// the main frame (frameID == "") and sub-frames.
	return resolveSelectorAtInFrame(ctx, frameID, selector.Selector{Kind: selector.KindCSS, Value: css}, 0, false)
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
		return 0, fmt.Errorf("ref %s not in snapshot cache: %w", sel.Value, ErrSelectorNoMatch)

	case selector.KindCSS:
		return ResolveCSSToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindXPath:
		return ResolveXPathToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindText:
		return ResolveTextToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")

	case selector.KindRole, selector.KindLabel, selector.KindPlaceholder,
		selector.KindAlt, selector.KindTitle, selector.KindTestID:
		return 0, fmt.Errorf("%s selectors must be resolved at the handler layer via semantic", sel.Kind)

	case selector.KindFirst:
		return resolveNestedSelectorAtInFrame(ctx, frameID, sel.Value, refCache, 0, false)

	case selector.KindLast:
		return resolveNestedSelectorAtInFrame(ctx, frameID, sel.Value, refCache, 0, true)

	case selector.KindNth:
		index, rawSelector, err := parseNthSelectorValue(sel.Value)
		if err != nil {
			return 0, err
		}
		return resolveNestedSelectorAtInFrame(ctx, frameID, rawSelector, refCache, index, false)

	default:
		return 0, fmt.Errorf("unknown selector kind: %q", sel.Kind)
	}
}

// ResolveUnifiedSelectorWithinNode resolves a selector strictly inside a DOM
// subtree. Ref selectors are accepted only when their cached backend node is
// still contained by that subtree; all other supported selectors are
// evaluated with the scope element as their root.
func ResolveUnifiedSelectorWithinNode(ctx context.Context, sel selector.Selector, refCache *RefCache, scopeBackendNodeID int64) (int64, error) {
	if scopeBackendNodeID == 0 {
		return 0, fmt.Errorf("dialog scope is missing")
	}

	switch sel.Kind {
	case selector.KindRef:
		if refCache == nil {
			return 0, fmt.Errorf("ref %s not in snapshot cache: %w", sel.Value, ErrSelectorNoMatch)
		}
		target, ok := refCache.Lookup(sel.Value)
		if !ok || target.BackendNodeID == 0 {
			return 0, fmt.Errorf("ref %s not in snapshot cache: %w", sel.Value, ErrSelectorNoMatch)
		}
		inside, err := BackendNodeWithinScope(ctx, scopeBackendNodeID, target.BackendNodeID)
		if err != nil {
			return 0, fmt.Errorf("validate ref %s against topmost dialog: %w", sel.Value, err)
		}
		if !inside {
			return 0, fmt.Errorf("ref %s is outside the topmost dialog: %w", sel.Value, ErrSelectorOutsideScope)
		}
		return target.BackendNodeID, nil

	case selector.KindCSS, selector.KindXPath, selector.KindText:
		return resolveSelectorAtWithinNode(ctx, scopeBackendNodeID, sel, 0, false)

	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")

	case selector.KindRole, selector.KindLabel, selector.KindPlaceholder,
		selector.KindAlt, selector.KindTitle, selector.KindTestID:
		return 0, fmt.Errorf("%s selectors must be resolved at the handler layer via semantic", sel.Kind)

	case selector.KindFirst:
		return resolveNestedSelectorWithinNode(ctx, scopeBackendNodeID, sel.Value, refCache, 0, false)

	case selector.KindLast:
		return resolveNestedSelectorWithinNode(ctx, scopeBackendNodeID, sel.Value, refCache, 0, true)

	case selector.KindNth:
		index, rawSelector, err := parseNthSelectorValue(sel.Value)
		if err != nil {
			return 0, err
		}
		return resolveNestedSelectorWithinNode(ctx, scopeBackendNodeID, rawSelector, refCache, index, false)

	default:
		return 0, fmt.Errorf("unknown selector kind: %q", sel.Kind)
	}
}

func ResolveUnifiedSelector(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	return ResolveUnifiedSelectorInFrame(ctx, sel, refCache, "")
}
