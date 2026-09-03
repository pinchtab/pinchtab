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

// writeIgnoredParamsComment is the same disclosure for the plain-text formats, in the
// comment shape those responses already use for hints.
func writeIgnoredParamsComment(w http.ResponseWriter, ignored []string) {
	if len(ignored) > 0 {
		_, _ = fmt.Fprintf(w, "# ignored params: %s\n", strings.Join(ignored, ", "))
	}
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
	defer h.armAutoCloseIfEnabled(resolvedTabID)
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
			scopedEmptyHint = fmt.Sprintf("Element exists in DOM (%s) but has no accessible nodes. Use `text --selector %s` or `eval` to extract content.", elemInfo, selector)
		} else if descErr == nil {
			scopedEmptyHint = fmt.Sprintf("Element exists in DOM but has no accessible nodes. Use `text --selector %s` or `eval` to extract content.", selector)
		}
	}

	truncated := false
	if maxTokens > 0 {
		flat, truncated = bridge.TruncateToTokens(flat, maxTokens, format)
	}

	prev := h.Bridge.GetRefCache(resolvedTabID)
	var prevNodes []bridge.A11yNode
	if doDiff && prev != nil {
		prevNodes = prev.Nodes
	}

	cache := bridge.EpochRefs(prev, flat)
	h.Bridge.SetRefCache(resolvedTabID, cache)
	w.Header().Set(vocabHeader, cache.DomEpoch)

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
			safe, err := httpx.SafeCreatePath(h.Config.StateDir, outputPath)
			if err != nil {
				httpx.Error(w, 400, fmt.Errorf("invalid path: %w", err))
				return
			}
			absBase, _ := filepath.Abs(h.Config.StateDir)
			absPath, err := filepath.Abs(safe)
			if err != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
				httpx.Error(w, 400, fmt.Errorf("invalid output path"))
				return
			}
			filePath = absPath
			if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
				httpx.Error(w, 500, fmt.Errorf("create output dir: %w", err))
				return
			}
			// A caller-named path keeps overwriting: this fix is about generated
			// default names, and a caller who names a file is entitled to replace it.
			if err := os.WriteFile(filePath, content, 0600); err != nil {
				httpx.Error(w, 500, fmt.Errorf("write snapshot: %w", err))
				return
			}
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
			_, _ = fmt.Fprintf(w, "%s | +%d ~%d -%d",
				snapshotCompactHeader(title, url, len(flat), scopeInfo), len(added), len(changed), len(removed))
			if truncated {
				_, _ = fmt.Fprintf(w, " (truncated to ~%d tokens)", maxTokens)
			}
			_, _ = w.Write([]byte("\n"))
			writeIgnoredParamsComment(w, controls.Ignored)
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
		_, _ = fmt.Fprintf(w, "%s", snapshotCompactHeader(title, url, len(flat), scopeInfo))
		if truncated {
			_, _ = fmt.Fprintf(w, " (truncated to ~%d tokens)", maxTokens)
		}
		_, _ = w.Write([]byte("\n"))
		if scopedEmptyHint != "" {
			_, _ = fmt.Fprintf(w, "# hint: %s\n", scopedEmptyHint)
		}
		writeIgnoredParamsComment(w, controls.Ignored)
		content := bridge.FormatSnapshotCompact(flat)
		if wrapContent {
			content = h.IDPIGuard.WrapContent(content, url)
		}
		_, _ = w.Write([]byte(content))
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "%s\n", snapshotTextHeader(title, url, len(flat), scopeInfo))
		if scopedEmptyHint != "" {
			_, _ = fmt.Fprintf(w, "# hint: %s\n", scopedEmptyHint)
		}
		writeIgnoredParamsComment(w, controls.Ignored)
		_, _ = w.Write([]byte("\n"))
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
