package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
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

func frameDocumentObjectID(ctx context.Context, frameID string) (string, error) {
	execID, err := cdptk.IsolatedContextID(ctx, frameID)
	if err != nil {
		return "", err
	}
	objectID, err := cdptk.EvaluateForHandle(ctx, execID, "document")
	if err != nil {
		return "", fmt.Errorf("resolve document: %w", err)
	}
	if objectID == "" {
		return "", fmt.Errorf("document object not found")
	}
	return objectID, nil
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

	objectID, err := cdptk.CallFunctionOnForHandle(ctx, docObjectID, functionDeclaration, args)
	if err != nil {
		return 0, err
	}
	if objectID == "" {
		return 0, fmt.Errorf("%w", ErrSelectorNoMatch)
	}
	return backendNodeIDFromObjectID(ctx, objectID)
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
	scopeObjectID, err := cdptk.IsolatedNodeObjectID(ctx, scopeBackendNodeID)
	if err != nil {
		return 0, fmt.Errorf("resolve scope node: %w", err)
	}

	objectID, err := cdptk.CallFunctionOnForHandle(ctx, scopeObjectID, functionDeclaration, args)
	if err != nil {
		return 0, fmt.Errorf("scoped selector evaluation: %w", err)
	}
	if objectID == "" {
		return 0, fmt.Errorf("%w", ErrSelectorNoMatch)
	}
	return backendNodeIDFromObjectID(ctx, objectID)
}

// BackendNodeWithinScope reports whether target is the scope node or one of
// its DOM descendants. It is used to reject stale/background snapshot refs
// while a modal dialog owns the interaction surface.
func BackendNodeWithinScope(ctx context.Context, scopeBackendNodeID, targetBackendNodeID int64) (bool, error) {
	if scopeBackendNodeID == 0 || targetBackendNodeID == 0 {
		return false, nil
	}

	objectIDs, err := cdptk.IsolatedNodeObjectIDs(ctx, scopeBackendNodeID, targetBackendNodeID)
	if err != nil {
		return false, fmt.Errorf("resolve scope and target nodes: %w", err)
	}
	scopeObjectID, targetObjectID := objectIDs[0], objectIDs[1]

	var contains bool
	err = cdptk.CallFunctionOn(ctx, scopeObjectID, `function(target) {
		for (let cur = target; cur; cur = cur.parentNode || cur.host || null) {
			if (cur === this) return true;
		}
		return false;
	}`, []map[string]any{{"objectId": targetObjectID}}, &contains)
	if err != nil {
		return false, fmt.Errorf("scope containment check: %w", err)
	}
	return contains, nil
}

func resolveElementMetaInFrame(ctx context.Context, frameID, functionDeclaration string, args []map[string]any) (FrameElementMeta, error) {
	docObjectID, err := frameDocumentObjectID(ctx, frameID)
	if err != nil {
		return FrameElementMeta{}, err
	}

	var meta *FrameElementMeta
	if err := cdptk.CallFunctionOn(ctx, docObjectID, functionDeclaration, args, &meta); err != nil {
		return FrameElementMeta{}, err
	}
	if meta == nil {
		return FrameElementMeta{}, fmt.Errorf("no element found")
	}
	meta.TagName = strings.ToLower(meta.TagName)
	return *meta, nil
}

// jsNormalizeHelper lowercases, collapses runs of whitespace to a single space,
// and trims. Indentation here is cosmetic (JS ignores it); only the runtime
// behavior matters.
const jsNormalizeHelper = `const normalize = (value) => String(value || "")
		.toLowerCase()
		.replace(/\s+/g, " ")
		.trim();`

func ResolveTextToNodeIDInFrame(ctx context.Context, frameID, text string) (int64, error) {
	return resolveSelectorAtInFrame(ctx, frameID, selector.Selector{Kind: selector.KindText, Value: text}, 0, false, false)
}

const resolveSelectorAtFn = `function(kind, value, index, fromEnd, positional) {
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
	const deepQueryAll = (selector, from) => {
		const out = [];
		const visit = (scope) => {
			if (!scope || !scope.querySelectorAll) return;
			if (scope.nodeType === 1 && scope.matches && scope.matches(selector)) out.push(scope);
			const elements = Array.from(scope.querySelectorAll("*"));
			out.push(...Array.from(scope.querySelectorAll(selector)));
			for (const el of elements) if (el.shadowRoot) visit(el.shadowRoot);
		};
		visit(from || root);
		return out;
	};
	// Source text is not rendered text: a CSS comment or a script string
	// mentioning a label is nothing a user can see or click, so matching it
	// hands back an element no action can operate on.
	const nonRendered = new Set(["script", "style", "noscript", "template"]);
	// At equal subtree size a control outranks a plain wrapper carrying the same
	// label, so a bare text: lands on the <button> rather than the <div> beside it.
	// This ranking is the BARE text: selector's rule only — a positional wrapper
	// indexes the document, like css: and xpath:.
	const semanticWeight = (el) => {
		const tag = (el.tagName || "").toLowerCase();
		if (tag === "button" || tag === "a" || tag === "input") return 0.25;
		const role = normalize(el.getAttribute && el.getAttribute("role"));
		if (role === "button" || role === "link" || role === "textbox") return 0.2;
		return 0;
	};
	// Leaf-most wins: the fewest descendants is the most specific match. This
	// REDUCES the match set, so it is shared by both paths — a wrapper chooses
	// among matches and must never change which matches exist. What survives stays
	// in document order, which is the order first:/last:/nth: index.
	const smallestMatches = (matches) => {
		const minSize = Math.min(...matches.map((item) => item.size));
		return matches
			.filter((item) => item.size === minSize)
			.map((item) => item.el);
	};
	// The bare text: selector's own rule, applied to the document-ordered set. It
	// has to pick the maximum weight EXPLICITLY: reading element zero of a
	// weight-sorted list is what made nth:1 resolve earlier in the document than
	// nth:0. Ties keep the first, so equal weights still mean document order.
	// A label that IS the query breaks a tie between equally control-like matches
	// — "Save" over "Save and exit" — but never outranks control-likeness: a
	// heading reading "Sign in" must not take the click from a "Sign in with
	// Google" button. A leaf inside a control weighs as that control, so the
	// <span>Save</span> of a button is not beaten by a bare "Save changes" button.
	const exactBonus = 0.01;
	const controlSelector = "button, a, input, [role=button], [role=link], [role=textbox]";
	const textWeight = (el, query) => {
		const control = (el.closest && el.closest(controlSelector)) || el;
		return semanticWeight(control) + (normalize(el.textContent || "") === query ? exactBonus : 0);
	};
	const bestRanked = (items, query) => {
		let best = items[0];
		let bestWeight = textWeight(best, query);
		for (const el of items) {
			const weight = textWeight(el, query);
			if (weight > bestWeight) {
				best = el;
				bestWeight = weight;
			}
		}
		return best;
	};
	const textCandidates = (query, exactFirst) => {
		if (!query) return [];
		// textContent, not innerText: innerText forces a synchronous layout per
		// element and is O(N^2) on large pages.
		const elements = deepQueryAll("*", root.body || root)
			.filter((el) => !nonRendered.has((el.tagName || "").toLowerCase()));
		const measured = elements.map((el) => ({ el, text: normalize(el.textContent || ""), size: el.getElementsByTagName("*").length }));
		// An element whose label IS the query beats one that merely contains it.
		// Without this rung the two were one filter — named exact, testing
		// includes — so "Save" and "Save and exit" were equally good answers to
		// text:Save, both leaf-most, both weighted as buttons, and the tie fell to
		// document order. A page listing the longer label first therefore sent
		// text:Delete to "Delete all" and reported OK.
		//
		// Containment still answers when nothing matches outright, so the
		// shorthand that reaches "Sign in" by text:Sign is untouched. This is the
		// ladder the select-option matcher already documents in
		// docs/reference/select.md: exact visible text, then substring.
		//
		// The rung applies to the positional wrappers, which index what it
		// returns. The bare selector takes every containing match and lets
		// bestRanked weigh exactness below control-likeness.
		if (exactFirst) {
			const equals = measured.filter((item) => item.text === query);
			if (equals.length) return smallestMatches(equals);
		}
		const contains = measured.filter((item) => item.text && item.text.includes(query));
		if (contains.length) return smallestMatches(contains);
		const tokens = query.split(" ").filter(Boolean);
		if (!tokens.length) return [];
		const fuzzy = measured.filter((item) => {
			if (!item.text) return false;
			let hits = 0;
			for (const token of tokens) if (item.text.includes(token)) hits++;
			return hits / tokens.length >= 0.7;
		});
		if (!fuzzy.length) return [];
		return smallestMatches(fuzzy);
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
		case "text": {
			const items = unique(textCandidates(needle, positional));
			if (!items.length) return null;
			return positional ? pick(items) : bestRanked(items, needle);
		}
		default:
			return null;
		}
	} catch (e) {
		return null;
	}
}`

// textLookupDeadline bounds the text scan so a slow resolution cannot eat the
// whole action timeout. It applies wherever text is matched, wrapped or not —
// first:/last:/nth: reach the same scan.
const textLookupDeadline = 3 * time.Second

// resolveSelectorAt is the single entry point for every css/xpath/text
// resolution. scopeLabel names the scope in errors; call runs the resolver
// against whichever root the caller resolved.
// positional says a first:/last:/nth: wrapper asked for this resolution. Plain
// text: and first:text: arrive with the same index and direction — 0 and forward —
// so the two cannot be told apart from the pair alone, and they must be: a bare
// text: ranks by control-likeness while a wrapper indexes the document.
func resolveSelectorAt(ctx context.Context, sel selector.Selector, index int, fromEnd bool, positional bool, scopeLabel string, call func(context.Context, []map[string]any) (int64, error)) (int64, error) {
	switch sel.Kind {
	case selector.KindCSS, selector.KindXPath, selector.KindText:
	default:
		return 0, fmt.Errorf("%s selector cannot be used with first/last/nth", sel.Kind)
	}

	lookupCtx := ctx
	if sel.Kind == selector.KindText {
		var cancel context.CancelFunc
		lookupCtx, cancel = context.WithTimeout(ctx, textLookupDeadline)
		defer cancel()
	}

	nid, err := call(lookupCtx, []map[string]any{
		{"value": string(sel.Kind)},
		{"value": sel.Value},
		{"value": index},
		{"value": fromEnd},
		{"value": positional},
	})
	if err != nil {
		if lookupCtx.Err() != nil && ctx.Err() == nil {
			return 0, fmt.Errorf("%s %q%s lookup timed out after %s (page may be large or unresponsive): %w", sel.Kind, sel.Value, scopeLabel, textLookupDeadline, err)
		}
		return 0, fmt.Errorf("%s %q%s: %w", sel.Kind, sel.Value, scopeLabel, err)
	}
	return nid, nil
}

func resolveSelectorAtInFrame(ctx context.Context, frameID string, sel selector.Selector, index int, fromEnd bool, positional bool) (int64, error) {
	return resolveSelectorAt(ctx, sel, index, fromEnd, positional, "", func(ctx context.Context, args []map[string]any) (int64, error) {
		var backendNodeID int64
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			nid, err := resolveNodeInFrame(ctx, frameID, resolveSelectorAtFn, args)
			if err != nil {
				return err
			}
			backendNodeID = nid
			return nil
		}))
		return backendNodeID, err
	})
}

func resolveSelectorAtWithinNode(ctx context.Context, scopeBackendNodeID int64, sel selector.Selector, index int, fromEnd bool, positional bool) (int64, error) {
	return resolveSelectorAt(ctx, sel, index, fromEnd, positional, " inside topmost dialog", func(ctx context.Context, args []map[string]any) (int64, error) {
		return resolveNodeWithinBackendNode(ctx, scopeBackendNodeID, resolveSelectorAtFn, args)
	})
}

// resolveScope is where a search is rooted. Selector grammar — unwrapping
// first:/last:/nth: — does not depend on it, so the recursion below is written
// once against this interface rather than once per scope kind.
type resolveScope interface {
	resolveAt(ctx context.Context, sel selector.Selector, index int, fromEnd bool, positional bool) (int64, error)
	resolveRef(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error)
}

type frameScope struct{ frameID string }

func (s frameScope) resolveAt(ctx context.Context, sel selector.Selector, index int, fromEnd bool, positional bool) (int64, error) {
	return resolveSelectorAtInFrame(ctx, s.frameID, sel, index, fromEnd, positional)
}

func (s frameScope) resolveRef(_ context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	if refCache != nil {
		if target, ok := refCache.Lookup(sel.Value); ok {
			return target.BackendNodeID, nil
		}
	}
	return 0, fmt.Errorf("ref %s not in snapshot cache: %w", sel.Value, ErrSelectorNoMatch)
}

type nodeScope struct{ backendNodeID int64 }

func (s nodeScope) resolveAt(ctx context.Context, sel selector.Selector, index int, fromEnd bool, positional bool) (int64, error) {
	return resolveSelectorAtWithinNode(ctx, s.backendNodeID, sel, index, fromEnd, positional)
}

// resolveRef differs from the frame scope only in containment. A cached ref still exists
// while the dialog owns the interaction surface, so it must be proven to lie
// inside the scope subtree — otherwise a dialog-scoped action silently reaches
// an element behind the dialog. The outside-scope sentinel is distinct from
// not-found: callers must not treat it as a stale ref.
func (s nodeScope) resolveRef(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	if refCache == nil {
		return 0, fmt.Errorf("ref %s not in snapshot cache: %w", sel.Value, ErrSelectorNoMatch)
	}
	target, ok := refCache.Lookup(sel.Value)
	if !ok {
		return 0, fmt.Errorf("ref %s not in snapshot cache: %w", sel.Value, ErrSelectorNoMatch)
	}
	inside, err := BackendNodeWithinScope(ctx, s.backendNodeID, target.BackendNodeID)
	if err != nil {
		return 0, fmt.Errorf("validate ref %s against topmost dialog: %w", sel.Value, err)
	}
	if !inside {
		return 0, fmt.Errorf("ref %s is outside the topmost dialog: %w", sel.Value, ErrSelectorOutsideScope)
	}
	return target.BackendNodeID, nil
}

func errSemanticAtResolver() error {
	return fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")
}

func errHandlerLayerKind(kind selector.Kind) error {
	return fmt.Errorf("%s selectors must be resolved at the handler layer via semantic", kind)
}

// resolveParsed is the one switch over selector kinds: it turns a
// first:/last:/nth: wrapper into the index/fromEnd pair the recursion carries, and
// hands anything else to the scope. The wrapper arms used to exist twice, split on
// whether the selector arrived parsed or raw — the grammar the scope refactor set
// out to stop duplicating was the part still duplicated. Parsing is now the only
// difference, and it lives in resolveNested.
//
// There is no "unknown selector kind" default. The old parsed-input copy had one and
// no input could reach it: both entry points dispatch here only from a
// case selector.KindFirst, selector.KindLast, selector.KindNth arm, and those arms
// are still the narrow kind check. Keeping a second switch alive to hold an
// unreachable error would be this duplication wearing a different hat.
func resolveParsed(ctx context.Context, scope resolveScope, sel selector.Selector, refCache *RefCache, index int, fromEnd bool, positional bool) (int64, error) {
	switch sel.Kind {
	case selector.KindFirst:
		return resolveNested(ctx, scope, sel.Value, refCache, 0, false, true)
	case selector.KindLast:
		return resolveNested(ctx, scope, sel.Value, refCache, 0, true, true)
	case selector.KindNth:
		nth, nestedRaw, err := selector.ParseNth(sel.Value)
		if err != nil {
			return 0, err
		}
		return resolveNested(ctx, scope, nestedRaw, refCache, nth, false, true)
	case selector.KindRef:
		if fromEnd || index != 0 {
			return 0, fmt.Errorf("ref selector cannot be used with last/nth")
		}
		return scope.resolveRef(ctx, sel, refCache)
	case selector.KindSemantic:
		return 0, errSemanticAtResolver()
	default:
		return scope.resolveAt(ctx, sel, index, fromEnd, positional)
	}
}

func resolveNested(ctx context.Context, scope resolveScope, raw string, refCache *RefCache, index int, fromEnd bool, positional bool) (int64, error) {
	return resolveParsed(ctx, scope, selector.Parse(raw), refCache, index, fromEnd, positional)
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
	return resolveSelectorAtInFrame(ctx, frameID, selector.Selector{Kind: selector.KindCSS, Value: css}, 0, false, false)
}

// XPath does not pierce shadow roots: document.evaluate cannot cross a shadow
// boundary. It resolves through the shared walker for consistency and
// isolated-world execution, not for shadow support.
func ResolveXPathToNodeIDInFrame(ctx context.Context, frameID, xpath string) (int64, error) {
	return resolveSelectorAtInFrame(ctx, frameID, selector.Selector{Kind: selector.KindXPath, Value: xpath}, 0, false, false)
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
		return frameScope{frameID}.resolveRef(ctx, sel, refCache)

	case selector.KindCSS:
		return ResolveCSSToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindXPath:
		return ResolveXPathToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindText:
		return ResolveTextToNodeIDInFrame(ctx, frameID, sel.Value)

	case selector.KindSemantic:
		return 0, errSemanticAtResolver()

	case selector.KindRole, selector.KindLabel, selector.KindPlaceholder,
		selector.KindAlt, selector.KindTitle, selector.KindTestID:
		return 0, errHandlerLayerKind(sel.Kind)

	case selector.KindFirst, selector.KindLast, selector.KindNth:
		return resolveParsed(ctx, frameScope{frameID}, sel, refCache, 0, false, false)

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
		return nodeScope{scopeBackendNodeID}.resolveRef(ctx, sel, refCache)

	case selector.KindCSS, selector.KindXPath, selector.KindText:
		return nodeScope{scopeBackendNodeID}.resolveAt(ctx, sel, 0, false, false)

	case selector.KindSemantic:
		return 0, errSemanticAtResolver()

	case selector.KindRole, selector.KindLabel, selector.KindPlaceholder,
		selector.KindAlt, selector.KindTitle, selector.KindTestID:
		return 0, errHandlerLayerKind(sel.Kind)

	case selector.KindFirst, selector.KindLast, selector.KindNth:
		return resolveParsed(ctx, nodeScope{scopeBackendNodeID}, sel, refCache, 0, false, false)

	default:
		return 0, fmt.Errorf("unknown selector kind: %q", sel.Kind)
	}
}
