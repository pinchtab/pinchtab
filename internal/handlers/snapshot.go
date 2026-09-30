package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	selectorpkg "github.com/pinchtab/pinchtab/internal/selector"
	"gopkg.in/yaml.v3"
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

func (h *Handlers) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
	controls, err := ParseSnapshotCostControls(r.URL.Query())
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	filter := controls.Filter

	tabID := r.URL.Query().Get("tabId")
	effectiveCfg, snapChromeRoute, ok := h.resolveReadRouting(w, r, tabID, "snapshot", browsers.ShapeStaticSnapshot)
	if !ok {
		return
	}

	if !h.ensureBrowserOrRespond(w, effectiveCfg) {
		return
	}

	doDiff := r.URL.Query().Get("diff") == "true"
	format := controls.Format
	output := r.URL.Query().Get("output")
	outputPath := r.URL.Query().Get("path")
	selector := r.URL.Query().Get("selector")
	reqNoAnim := r.URL.Query().Get("noAnimations") == "true"
	maxDepth := controls.MaxDepth
	maxTokens := controls.MaxTokens
	wireCarriesMetadata := snapshotFormatCarriesMetadata(format)

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, tabID, effectiveCfg.ActionTimeout)
	if !ok {
		return
	}
	defer h.armIdleLifecycle(resolvedTabID)
	defer cancel()

	if reqNoAnim && !h.Config.NoAnimations {
		if err := bridge.DisableAnimationsOnce(tCtx); err != nil {
			httpx.Error(w, 500, fmt.Errorf("disable animations: %w", err))
			return
		}
	}

	var flat []bridge.A11yNode
	var url, title string
	var scopeNodeID int64

	frameScope := h.selectorFrameID(resolvedTabID)
	scopeInfo := h.frameDisclosureFor(tCtx, resolvedTabID, frameScope)
	ghostRoute := snapChromeRoute != nil && snapChromeRoute.UsedBrowser == config.BrowserGhostChrome
	var modalOpen bool
	if frameScope != "" || selector != "" || !ghostRoute {
		// Frame-scoped or selector-scoped: inline AX tree fetch with scoping.
		var rawNodes []bridge.RawAXNode
		stable := false
		for attempt := 0; attempt < 2; attempt++ {
			var modalNodeID int64
			var modalErr error
			if !ghostRoute {
				modalNodeID, modalOpen, modalErr = bridge.TopmostModalNodeID(tCtx, frameScope)
				if modalErr != nil {
					respondSelectorFailure(w, modalErr)
					return
				}
			}

			candidateNodes, candidateScope, scopeErr := h.scopedSnapshotNodes(
				tCtx, resolvedTabID, frameScope, selector, modalNodeID, modalOpen,
			)
			if !ghostRoute {
				afterNodeID, afterOpen, recheckErr := bridge.TopmostModalNodeID(tCtx, frameScope)
				if recheckErr != nil {
					respondSelectorFailure(w, fmt.Errorf("recheck topmost dialog: %w", recheckErr))
					return
				}
				if modalNodeID != afterNodeID || modalOpen != afterOpen {
					continue
				}
			}
			if scopeErr != nil {
				respondSelectorFailure(w, scopeErr)
				return
			}
			rawNodes, scopeNodeID, stable = candidateNodes, candidateScope, true
			break
		}
		if !stable {
			httpx.Error(w, http.StatusConflict, fmt.Errorf("topmost dialog changed twice during snapshot; retry after the page settles"))
			return
		}

		flat, _ = bridge.BuildSnapshot(rawNodes, filter, maxDepth)
		if wireCarriesMetadata {
			_ = bridge.EnrichA11yNodesWithDOMMetadata(tCtx, flat)
		}
		url, _ = h.Bridge.CurrentURL(tCtx)
		title, _ = h.Bridge.CurrentTitle(tCtx)
	} else {
		// Unscoped: delegate to Bridge (enables ghost-chrome routing via BridgeAdapter).
		result, err := h.Bridge.Snapshot(tCtx, resolvedTabID, filter, bridge.ContentParams{
			MaxDepth:     maxDepth,
			SkipMetadata: !wireCarriesMetadata,
		})
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("snapshot: %w", err))
			return
		}
		flat = result.Nodes
		url = result.URL
		title = result.Title
		if result.Route != nil {
			snapChromeRoute = result.Route
		}
	}

	var scopedEmptyHint string
	if len(flat) == 0 && selector != "" && scopeNodeID != 0 {
		var elemInfo string
		nodeInfo, descErr := h.Bridge.DescribeNode(tCtx, scopeNodeID)
		if descErr == nil && nodeInfo != nil {
			tag := nodeInfo.LocalName
			childCount := nodeInfo.ChildNodeCount
			attrs := ""
			for i := 0; i+1 < len(nodeInfo.Attributes); i += 2 {
				switch nodeInfo.Attributes[i] {
				case "id":
					attrs += "#" + nodeInfo.Attributes[i+1]
				case "class":
					classes := strings.Fields(nodeInfo.Attributes[i+1])
					if len(classes) > 0 {
						attrs += "." + strings.Join(classes[:min(2, len(classes))], ".")
					}
				}
			}
			if tag != "" {
				elemInfo = fmt.Sprintf("<%s%s> with %d child nodes", tag, attrs, childCount)
			}
		}
		if elemInfo != "" {
			scopedEmptyHint = fmt.Sprintf("Element exists in DOM (%s) but has no accessible nodes. Use `text` with the same selector, or `eval`, to extract content.", elemInfo)
		} else if descErr == nil {
			scopedEmptyHint = "Element exists in DOM but has no accessible nodes. Use `text` with the same selector, or `eval`, to extract content."
		}
	}

	truncated := false
	if maxTokens > 0 {
		// The budget has to cover the reply, not just the part of it made of nodes.
		// TruncateToTokens was fitting the tree to maxTokens and the header was then
		// written on top of it, so what the caller received was over by whatever the
		// header cost — and the header carries the title and the URL, so that is a
		// page-dependent amount, not a constant to shrug at. Measured on a page with
		// an ordinary marketing title and path, a budget of 100 returned ~142 tokens.
		//
		// Reserved before allocating, with truncation assumed: a budget is a ceiling,
		// so where the two readings differ this takes the larger one. The node count
		// in the header is the pre-truncation one for the same reason — it has at
		// least as many digits as the count that will be printed.
		nodeBudget := h.snapshotNodeBudget(format, output, title, url, len(flat), scopeInfo, scopedEmptyHint, controls.Ignored, maxTokens)
		flat, truncated = bridge.TruncateToTokens(flat, nodeBudget, format)
	}

	prev := h.Bridge.GetRefCache(resolvedTabID)
	var prevNodes []bridge.A11yNode
	if doDiff && prev != nil {
		prevNodes = prev.Nodes
	}

	cache := bridge.EpochRefs(prev, flat)
	h.Bridge.SetRefCache(resolvedTabID, cache)
	publishVocab(w, resolvedTabID, cache.DomEpoch)

	h.recordResolvedURL(r, url)

	// IDPI: scan accessibility-tree node names and values for injection patterns.
	// The scan runs after the snapshot is built so truncation has already reduced
	// the corpus. Headers are set before any write so they always reach the client.
	idpiResult := h.scanSnapshotIDPI(w, flat)
	if idpiResult.Blocked {
		return
	}
	wrapContent := idpiResult.WrapContent

	if output == "file" {
		snapshotDir := filepath.Join(h.Config.StateDir, "snapshots")
		if err := os.MkdirAll(snapshotDir, 0750); err != nil {
			httpx.Error(w, 500, fmt.Errorf("create snapshot dir: %w", err))
			return
		}

		timestamp := exportTimestamp()
		var ext string
		var content []byte

		switch format {
		case "text":
			ext = ".txt"
			textContent := fmt.Sprintf("%s\n# %s\n\n%s",
				snapshotTextHeader(title, url, len(flat), scopeInfo), time.Now().Format(time.RFC3339),
				bridge.FormatSnapshotText(flat))
			content = []byte(textContent)
		case "yaml":
			ext = ".yaml"
			data := scopeInfo.attach(map[string]any{
				"url":       url,
				"title":     title,
				"timestamp": time.Now().Format(time.RFC3339),
				"nodes":     flat,
				"count":     len(flat),
			})
			if doDiff && prevNodes != nil {
				added, changed, removed := bridge.DiffSnapshot(prevNodes, flat)
				data["diff"] = true
				data["added"] = added
				data["changed"] = changed
				data["removed"] = removed
				data["counts"] = map[string]int{
					"added":   len(added),
					"changed": len(changed),
					"removed": len(removed),
					"total":   len(flat),
				}
			}
			var err error
			content, err = yaml.Marshal(data)
			if err != nil {
				httpx.Error(w, 500, fmt.Errorf("marshal yaml: %w", err))
				return
			}
		default:
			ext = ".json"
			data := scopeInfo.attach(map[string]any{
				"url":       url,
				"title":     title,
				"timestamp": time.Now().Format(time.RFC3339),
				"nodes":     flat,
				"count":     len(flat),
			})
			if doDiff && prevNodes != nil {
				added, changed, removed := bridge.DiffSnapshot(prevNodes, flat)
				data["diff"] = true
				data["added"] = added
				data["changed"] = changed
				data["removed"] = removed
				data["counts"] = map[string]int{
					"added":   len(added),
					"changed": len(changed),
					"removed": len(removed),
					"total":   len(flat),
				}
			}
			var err error
			content, err = json.MarshalIndent(data, "", "  ")
			if err != nil {
				httpx.Error(w, 500, fmt.Errorf("marshal snapshot: %w", err))
				return
			}
		}

		var filePath string
		if outputPath != "" {
			p, status, err := saveToCallerPath(h.Config.StateDir, outputPath, content)
			if err != nil {
				httpx.Error(w, status, fmt.Errorf("snapshot: %w", err))
				return
			}
			filePath = p
		} else {
			var err error
			filePath, err = writeUniqueFile(snapshotDir, "snapshot-"+timestamp, ext, content)
			if err != nil {
				httpx.Error(w, 500, fmt.Errorf("write snapshot: %w", err))
				return
			}
		}

		httpx.JSON(w, 200, attachIgnoredParams(map[string]any{
			"path":      filePath,
			"size":      len(content),
			"format":    format,
			"timestamp": timestamp,
		}, controls.Ignored))
		return
	}

	if doDiff && prevNodes != nil {
		added, changed, removed := bridge.DiffSnapshot(prevNodes, flat)

		// Compact diff format: show all nodes with [+]/[~] markers
		if format == "compact" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(200)
			diffHeader := fmt.Sprintf("%s | +%d ~%d -%d",
				snapshotCompactHeader(title, url, len(flat), scopeInfo), len(added), len(changed), len(removed))
			_, _ = w.Write([]byte(snapshotFraming(format, diffHeader, "", controls.Ignored, truncated, maxTokens)))
			content := bridge.FormatSnapshotCompactDiff(flat, added, changed, removed)
			if wrapContent {
				content = h.IDPIGuard.WrapContent(content, url)
			}
			_, _ = w.Write([]byte(content))
			return
		}

		httpx.JSON(w, 200, attachIgnoredParams(scopeInfo.attach(map[string]any{
			"url":     url,
			"title":   title,
			"route":   snapChromeRoute,
			"diff":    true,
			"added":   added,
			"changed": changed,
			"removed": removed,
			"counts": map[string]int{
				"added":   len(added),
				"changed": len(changed),
				"removed": len(removed),
				"total":   len(flat),
			},
		}), controls.Ignored))
		return
	}

	switch format {
	case "compact":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(snapshotFraming(format,
			snapshotCompactHeader(title, url, len(flat), scopeInfo),
			scopedEmptyHint, controls.Ignored, truncated, maxTokens)))
		content := bridge.FormatSnapshotCompact(flat)
		if wrapContent {
			content = h.IDPIGuard.WrapContent(content, url)
		}
		_, _ = w.Write([]byte(content))
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(snapshotFraming(format,
			snapshotTextHeader(title, url, len(flat), scopeInfo),
			scopedEmptyHint, controls.Ignored, truncated, maxTokens)))
		content := bridge.FormatSnapshotText(flat)
		if wrapContent {
			content = h.IDPIGuard.WrapContent(content, url)
		}
		_, _ = w.Write([]byte(content))
	case "yaml":
		data := scopeInfo.attach(map[string]any{
			"url":   url,
			"title": title,
			"nodes": flat,
			"count": len(flat),
		})
		if scopedEmptyHint != "" {
			data["hint"] = scopedEmptyHint
		}
		attachIgnoredParams(data, controls.Ignored)
		yamlContent, err := yaml.Marshal(data)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("marshal yaml: %w", err))
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write(yamlContent)
	default:
		resp := scopeInfo.attach(map[string]any{
			"url":             url,
			"title":           title,
			"route":           snapChromeRoute,
			"nodes":           flat,
			"count":           len(flat),
			"vocabularyToken": cache.DomEpoch,
		})
		if truncated {
			resp["truncated"] = true
			resp["maxTokens"] = maxTokens
		}
		if scopedEmptyHint != "" {
			resp["hint"] = scopedEmptyHint
		}
		attachIgnoredParams(resp, controls.Ignored)
		if idpiResult.Threat {
			resp["idpiWarning"] = idpiResult.Reason
		}
		if wrapContent {
			resp["untrustedContent"] = true
			resp["idpiNotice"] = idpiNoticeText
		}
		httpx.JSON(w, 200, resp)
	}
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
