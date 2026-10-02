package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	selectorpkg "github.com/pinchtab/pinchtab/internal/selector"
)

// HandleSnapshot returns the accessibility tree of a tab.
//
// @Endpoint GET /snapshot
// @Description Returns the page structure with clickable elements, form fields, and text content
//
// @Param tabId string query Tab ID (required)
// @Param filter string query Filter type: "interactive" for clickable/inputs only, "all" for everything (optional, default: "all")
// @Param interactive bool query Alias for filter: true means filter=interactive, false means filter=all; a value contradicting an explicit filter is a 400 (optional)
// @Param depth int query Max nesting depth (optional, default: -1 for full tree)
// @Param maxTokens int query Token budget for the response (optional, positive)
// @Param format string query Output format: "json", "compact", "text" or "yaml" (optional, default: "json")
// @Param diff bool query Include diff with previous snapshot (optional, default: false)
// @Param output string query Write to file instead of response (optional)
//
// @Response 200 application/json Returns accessibility tree with refs
// @Response 400 application/json Invalid tabId or parameters
// @Response 404 application/json Tab not found
//
// @Example curl all elements:
//
//	curl "http://localhost:9867/snapshot?tabId=abc123"
//
// @Example curl interactive only:
//
//	curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive"
//
// @Example curl compact:
//
//	curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive&format=compact"
//
// @Example cli:
//
//	pinchtab snap -i -c
//
// @Example python:
//
//	import requests
//	r = requests.get("http://localhost:9867/snapshot", params={"tabId": "abc123", "filter": "interactive"})
//	tree = r.json()
//
// attachIgnoredParams reports the query parameters the server did not read. A caller that
// mistypes a flag otherwise gets a plausible-looking answer to a question it did not ask —
// which is exactly how `compact=true` survived in the CLI for as long as it did.
func attachIgnoredParams(data map[string]any, ignored []string) map[string]any {
	if len(ignored) > 0 {
		data["ignoredParams"] = ignored
	}
	return data
}

// estimateSnapshotTokens converts framing bytes into the same tokens the truncator
// charges nodes in. Four bytes per token is the approximation the budget rests on,
// and the reserve has to be denominated in it or the two halves of the ceiling
// would be measured in different units.
//
// It rounds UP where the truncator's charge rounds down, and the asymmetry is the
// point: a charge that rounds down can spend a fraction of a token it did not pay
// for, while a reserve that rounds down leaves a fraction of the framing unfunded.
// Both were floors to begin with, and the two remainders met — framing of 44.75
// tokens reserved 44, and a 100-token budget returned 101.
func estimateSnapshotTokens(bytes int) int {
	return (bytes + 3) / 4
}

// snapshotFramingReserve is what this reply costs before a node is written. The
// untrusted-content wrapper is included when the configuration turns it on: it is
// decided by config rather than by the scan — scanSnapshotIDPI reads
// IDPI.Enabled && IDPI.WrapContent — so it is knowable here, ahead of the scan
// that deliberately runs after truncation. It is also the larger of the two
// framings, being a fixed advisory paragraph.
func (h *Handlers) snapshotFramingReserve(format, title, url string, count int, scope *frameDisclosure, hint string, ignored []string, maxTokens int) int {
	var header string
	switch format {
	case "text":
		header = snapshotTextHeader(title, url, count, scope)
	default:
		header = snapshotCompactHeader(title, url, count, scope)
	}
	n := len(snapshotFraming(format, header, hint, ignored, true, maxTokens))
	if h.Config != nil && h.Config.IDPI.Enabled && h.Config.IDPI.WrapContent && h.IDPIGuard != nil {
		n += len(h.IDPIGuard.WrapContent("", url))
	}
	return n
}

// snapshotFramingIsReserved reports whether the reply carries the plain-text framing
// the reserve prices. json and yaml carry no header or advisory — only the node
// array is budgeted — and a file export writes its own layout.
func snapshotFramingIsReserved(format, output string) bool {
	return output != "file" && (format == "compact" || format == "text")
}

// snapshotNodeBudget is what is left of maxTokens for nodes once the framing is paid
// for. It holds back one token more than the framing: the truncator rounds its
// charge down, so it can spend up to three bytes past the budget it is handed.
func (h *Handlers) snapshotNodeBudget(format, output, title, url string, count int, scope *frameDisclosure, hint string, ignored []string, maxTokens int) int {
	if !snapshotFramingIsReserved(format, output) {
		return maxTokens
	}
	reserve := estimateSnapshotTokens(h.snapshotFramingReserve(format, title, url, count, scope, hint, ignored, maxTokens))
	return max(maxTokens-reserve-1, 0)
}

func ignoredParamsComment(ignored []string) string {
	if len(ignored) == 0 {
		return ""
	}
	return fmt.Sprintf("# ignored params: %s\n", strings.Join(ignored, ", "))
}

// snapshotFraming is everything the caller is sent that is not a node: the header,
// the hint, the ignored-params comment, and the blank line the text layout puts
// between them and the tree.
//
// It is built once and used twice — measured to reserve budget, then written — so
// the charge cannot drift from the output. Building a second model of the framing
// in order to price it is the mistake nodeCost already avoids by rendering through
// appendNode rather than describing it.
func snapshotFraming(format, header, hint string, ignored []string, truncated bool, maxTokens int) string {
	var b strings.Builder
	b.WriteString(header)
	// Only the compact layout annotates the header with the budget; text does not,
	// and this reproduces each exactly rather than making them agree.
	if truncated && format != "text" {
		fmt.Fprintf(&b, " (truncated to ~%d tokens)", maxTokens)
	}
	b.WriteString("\n")
	if hint != "" {
		fmt.Fprintf(&b, "# hint: %s\n", hint)
	}
	b.WriteString(ignoredParamsComment(ignored))
	if format == "text" {
		b.WriteString("\n")
	}
	return b.String()
}

func snapshotFormatCarriesMetadata(format string) bool {
	return format == "json" || format == "yaml"
}

func snapshotEmitsWireNodes(q snapshotQuery) bool {
	return snapshotFormatCarriesMetadata(q.format) || (q.output == "file" && q.format != "text")
}

type snapshotQuery struct {
	controls     SnapshotCostControls
	tabID        string
	format       string
	output       string
	outputPath   string
	selector     string
	doDiff       bool
	noAnimations bool
}

type snapshotResult struct {
	flat        []bridge.A11yNode
	rootFrameID string
	url         string
	title       string
	scopeNodeID int64
	scope       *frameDisclosure
	route       *browserops.RouteMetadata
	hint        string
	truncated   bool
}

func parseSnapshotQuery(r *http.Request) (snapshotQuery, error) {
	q := r.URL.Query()
	controls, err := ParseSnapshotCostControls(q)
	if err != nil {
		return snapshotQuery{}, err
	}
	return snapshotQuery{
		controls:     controls,
		tabID:        q.Get("tabId"),
		format:       controls.Format,
		output:       q.Get("output"),
		outputPath:   q.Get("path"),
		selector:     q.Get("selector"),
		doDiff:       q.Get("diff") == "true",
		noAnimations: q.Get("noAnimations") == "true",
	}, nil
}

func (h *Handlers) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
	q, err := parseSnapshotQuery(r)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}

	effectiveCfg, route, ok := h.resolveReadRouting(w, r, q.tabID, "snapshot", browsers.ShapeStaticSnapshot)
	if !ok {
		return
	}
	if !h.ensureBrowserOrRespond(w, effectiveCfg) {
		return
	}

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, q.tabID, effectiveCfg.ActionTimeout)
	if !ok {
		return
	}
	defer h.armIdleLifecycle(resolvedTabID)
	defer cancel()

	if q.noAnimations && !h.Config.NoAnimations {
		if err := bridge.DisableAnimationsOnce(tCtx); err != nil {
			httpx.Error(w, 500, fmt.Errorf("disable animations: %w", err))
			return
		}
	}

	res, ok := h.collectSnapshot(w, tCtx, resolvedTabID, q, route)
	if !ok {
		return
	}
	res.hint = h.scopedEmptyHint(tCtx, res.flat, q.selector, res.scopeNodeID)
	h.applySnapshotBudget(q, &res)

	prev := h.Bridge.GetRefCache(resolvedTabID)
	var prevNodes []bridge.A11yNode
	if q.doDiff && prev != nil {
		prevNodes = prev.Nodes
	}
	cache := bridge.EpochRefs(prev, res.flat)
	h.Bridge.SetRefCache(resolvedTabID, cache)
	publishVocab(w, resolvedTabID, cache.DomEpoch)
	h.recordResolvedURL(r, res.url)

	idpiResult := h.scanSnapshotIDPI(w, res.flat)
	if idpiResult.Blocked {
		return
	}

	switch {
	case q.output == "file":
		h.writeSnapshotFile(w, q, res, prevNodes)
	case q.doDiff && prevNodes != nil:
		h.writeSnapshotDiff(w, q, res, prevNodes, idpiResult.WrapContent)
	default:
		h.writeSnapshot(w, q, res, cache.DomEpoch, idpiResult)
	}
}

func (h *Handlers) collectSnapshot(w http.ResponseWriter, tCtx context.Context, tabID string, q snapshotQuery, route *browserops.RouteMetadata) (snapshotResult, bool) {
	res := snapshotResult{route: route}
	frameScope := h.selectorFrameID(tabID)
	res.scope = h.frameDisclosureFor(tCtx, tabID, frameScope)
	wireCarriesMetadata := snapshotFormatCarriesMetadata(q.format)
	ghostRoute := route != nil && route.UsedBrowser == config.BrowserGhostChrome

	if frameScope == "" && q.selector == "" && ghostRoute {
		result, err := h.Bridge.Snapshot(tCtx, tabID, q.controls.Filter, bridge.ContentParams{
			MaxDepth:     q.controls.MaxDepth,
			SkipMetadata: !wireCarriesMetadata,
		})
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("snapshot: %w", err))
			return res, false
		}
		res.flat, res.url, res.title = result.Nodes, result.URL, result.Title
		if result.Route != nil {
			res.route = result.Route
		}
		if snapshotEmitsWireNodes(q) {
			res.rootFrameID = snapshotRootFrameID(tCtx, res.flat)
		}
		return res, true
	}

	var rawNodes []bridge.RawAXNode
	workErr, scopeErr := withStableModalScope(tCtx, frameScope, ghostRoute, func(modalNodeID int64, modalOpen bool) error {
		var err error
		rawNodes, res.scopeNodeID, err = h.scopedSnapshotNodes(tCtx, tabID, frameScope, q.selector, modalNodeID, modalOpen)
		return err
	})
	if errors.Is(scopeErr, errModalUnstable) {
		httpx.Error(w, http.StatusConflict, fmt.Errorf("topmost dialog changed twice during snapshot; retry after the page settles"))
		return res, false
	}
	if err := cmp.Or(scopeErr, workErr); err != nil {
		h.respondSelectorFailure(w, tabID, err)
		return res, false
	}

	res.flat, _ = bridge.BuildSnapshot(rawNodes, q.controls.Filter, q.controls.MaxDepth)
	if wireCarriesMetadata {
		_ = bridge.EnrichA11yNodesWithDOMMetadata(tCtx, res.flat)
	}
	if snapshotEmitsWireNodes(q) {
		res.rootFrameID = snapshotRootFrameID(tCtx, res.flat)
	}
	res.url, _ = h.Bridge.CurrentURL(tCtx)
	res.title, _ = h.Bridge.CurrentTitle(tCtx)
	return res, true
}

func snapshotRootFrameID(ctx context.Context, flat []bridge.A11yNode) string {
	frameTagged := slices.ContainsFunc(flat, func(n bridge.A11yNode) bool { return n.FrameID != "" })
	if !frameTagged {
		return ""
	}
	tree, err := observe.FetchFrameTree(ctx)
	if err != nil {
		return ""
	}
	return tree.Frame.ID
}

func (h *Handlers) scopedEmptyHint(tCtx context.Context, flat []bridge.A11yNode, selector string, scopeNodeID int64) string {
	if len(flat) != 0 || selector == "" || scopeNodeID == 0 {
		return ""
	}
	nodeInfo, err := h.Bridge.DescribeNode(tCtx, scopeNodeID)
	if err != nil {
		return ""
	}
	if nodeInfo != nil && nodeInfo.LocalName != "" {
		var attrs strings.Builder
		for i := 0; i+1 < len(nodeInfo.Attributes); i += 2 {
			switch nodeInfo.Attributes[i] {
			case "id":
				attrs.WriteString("#" + nodeInfo.Attributes[i+1])
			case "class":
				classes := strings.Fields(nodeInfo.Attributes[i+1])
				if len(classes) > 0 {
					attrs.WriteString("." + strings.Join(classes[:min(2, len(classes))], "."))
				}
			}
		}
		elemInfo := fmt.Sprintf("<%s%s> with %d child nodes", nodeInfo.LocalName, attrs.String(), nodeInfo.ChildNodeCount)
		return fmt.Sprintf("Element exists in DOM (%s) but has no accessible nodes. Use `text` with the same selector, or `eval`, to extract content.", elemInfo)
	}
	return "Element exists in DOM but has no accessible nodes. Use `text` with the same selector, or `eval`, to extract content."
}

// applySnapshotBudget fits the whole reply, header included, under maxTokens. The
// header carries the page title and URL, so its cost is reserved before the nodes are
// cut, using the pre-truncation count so the reservation is never short.
func (h *Handlers) applySnapshotBudget(q snapshotQuery, res *snapshotResult) {
	if q.controls.MaxTokens <= 0 {
		return
	}
	nodeBudget := h.snapshotNodeBudget(q.format, q.output, res.title, res.url, len(res.flat), res.scope, res.hint, q.controls.Ignored, q.controls.MaxTokens)
	res.flat, res.truncated = bridge.TruncateToTokens(res.flat, nodeBudget, q.format)
}

// snapshotCompactHeader and snapshotTextHeader are the one place each header shape is built,
// so the scope marker cannot reach three of the four sites that print one. The marker keeps
// title and url meaning what they always meant — the TAB's document — and adds the fact that
// the nodes below came from a frame inside it; re-pointing url at the frame would make one
// field mean two things depending on invisible state, which is the defect being fixed.
func snapshotCompactHeader(title, url string, count int, scope *frameDisclosure) string {
	parts := []string{"# " + title, url}
	if marker := scope.marker(); marker != "" {
		parts = append(parts, marker)
	}
	return strings.Join(append(parts, fmt.Sprintf("%d nodes", count)), " | ")
}

func snapshotTextHeader(title, url string, count int, scope *frameDisclosure) string {
	header := fmt.Sprintf("# %s\n# %s\n", title, url)
	if marker := scope.marker(); marker != "" {
		header += "# " + marker + "\n"
	}
	return header + fmt.Sprintf("# %d nodes", count)
}

func (h *Handlers) scopedSnapshotNodes(
	ctx context.Context,
	tabID, frameScope, rawSelector string,
	modalNodeID int64,
	modalOpen bool,
) ([]bridge.RawAXNode, int64, error) {
	rawNodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("a11y tree: %w", err)
	}
	if !modalOpen {
		rawNodes = bridge.FilterAXNodesByFrame(rawNodes, frameScope)
	}
	scopeNodeID := int64(0)
	if modalOpen {
		if !axTreeContainsBackendNode(rawNodes, modalNodeID) {
			return nil, 0, fmt.Errorf("topmost dialog is absent from the accessibility tree")
		}
		rawNodes = bridge.FilterSubtree(rawNodes, modalNodeID)
		scopeNodeID = modalNodeID
	}

	if rawSelector == "" {
		return rawNodes, scopeNodeID, nil
	}
	if modalOpen {
		scopeNodeID, err = bridge.ResolveUnifiedSelectorWithinNode(ctx, selectorpkg.Parse(rawSelector), h.Bridge.GetRefCache(tabID), modalNodeID)
	} else {
		scopeNodeID, err = h.resolveSelectorNodeID(ctx, tabID, rawSelector)
	}
	if err != nil {
		return nil, 0, frameScopedSelectorError("selector", err)
	}
	if !axTreeContainsBackendNode(rawNodes, scopeNodeID) {
		// A valid DOM element can be absent from the accessibility tree. Return
		// an empty scoped result so FilterSubtree's legacy not-found fallback
		// cannot expose the surrounding modal or page.
		return nil, scopeNodeID, nil
	}
	return bridge.FilterSubtree(rawNodes, scopeNodeID), scopeNodeID, nil
}

func axTreeContainsBackendNode(nodes []bridge.RawAXNode, backendNodeID int64) bool {
	for _, node := range nodes {
		if node.BackendDOMNodeID == backendNodeID {
			return true
		}
	}
	return false
}

// @Endpoint GET /tabs/{id}/snapshot
func (h *Handlers) HandleTabSnapshot(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleSnapshot)
}
