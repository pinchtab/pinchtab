package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"gopkg.in/yaml.v3"
)

// HandleSnapshot returns the accessibility tree of a tab.
//
// @Endpoint GET /snapshot
// @Description Returns the page structure with clickable elements, form fields, and text content
//
// @Param tabId string query Tab ID (required)
// @Param filter string query Filter type: "interactive" for clickable/inputs only, "all" for everything (optional, default: "all")
// @Param interactive bool query Alias for filter=interactive (optional)
// @Param compact bool query Compact output (shorter ref names) (optional, default: false)
// @Param depth int query Max nesting depth (optional, default: -1 for full tree)
// @Param text bool query Include text content (optional, default: true)
// @Param format string query Output format: "json" or "yaml" (optional, default: "json")
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
//	curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive&compact=true"
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
func (h *Handlers) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")

	tabID := r.URL.Query().Get("tabId")
	effectiveCfg, snapChromeRoute, ok := h.resolveReadRouting(w, r, tabID, "snapshot", browsers.ShapeStaticSnapshot)
	if !ok {
		return
	}

	if !h.ensureBrowserOrRespond(w, effectiveCfg) {
		return
	}

	doDiff := r.URL.Query().Get("diff") == "true"
	format := r.URL.Query().Get("format")
	output := r.URL.Query().Get("output")
	outputPath := r.URL.Query().Get("path")
	selector := r.URL.Query().Get("selector")
	maxTokensStr := r.URL.Query().Get("maxTokens")
	reqNoAnim := r.URL.Query().Get("noAnimations") == "true"
	maxDepthStr := r.URL.Query().Get("depth")
	maxDepth := -1
	if maxDepthStr != "" {
		if d, err := strconv.Atoi(maxDepthStr); err == nil {
			maxDepth = d
		}
	}
	maxTokens := -1
	if maxTokensStr != "" {
		if t, err := strconv.Atoi(maxTokensStr); err == nil && t > 0 {
			maxTokens = t
		}
	}

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
	var refs map[string]int64
	var url, title string
	var scopeNodeID int64

	frameScope := h.selectorFrameID(resolvedTabID)
	if frameScope != "" || selector != "" {
		// Frame-scoped or selector-scoped: inline AX tree fetch with scoping.
		rawNodes, err := bridge.FetchAXTree(tCtx)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("a11y tree: %w", err))
			return
		}
		rawNodes = bridge.FilterAXNodesByFrame(rawNodes, frameScope)

		if selector != "" {
			var scopeErr error
			scopeNodeID, scopeErr = h.resolveSelectorNodeID(tCtx, resolvedTabID, selector)
			if scopeErr != nil {
				httpx.Error(w, 400, frameScopedSelectorError("selector", scopeErr))
				return
			}
			rawNodes = bridge.FilterSubtree(rawNodes, scopeNodeID)
		}

		flat, refs = bridge.BuildSnapshot(rawNodes, filter, maxDepth)
		_ = bridge.EnrichA11yNodesWithDOMMetadata(tCtx, flat)
		url, _ = h.Bridge.CurrentURL(tCtx)
		title, _ = h.Bridge.CurrentTitle(tCtx)
	} else {
		// Unscoped: delegate to Bridge (enables ghost-chrome routing via BridgeAdapter).
		result, err := h.Bridge.Snapshot(tCtx, resolvedTabID, filter, bridge.ContentParams{
			MaxDepth: maxDepth,
		})
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("snapshot: %w", err))
			return
		}
		flat = result.Nodes
		refs = result.Refs
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

	var prevNodes []bridge.A11yNode
	if doDiff {
		if prev := h.Bridge.GetRefCache(resolvedTabID); prev != nil {
			prevNodes = prev.Nodes
		}
	}

	h.Bridge.SetRefCache(resolvedTabID, &bridge.RefCache{
		Refs:    refs,
		Targets: bridge.RefTargetsFromNodes(flat),
		Nodes:   flat,
	})

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

		timestamp := time.Now().Format("20060102-150405")
		var filename string
		var content []byte

		switch format {
		case "text":
			filename = fmt.Sprintf("snapshot-%s.txt", timestamp)
			textContent := fmt.Sprintf("# %s\n# %s\n# %d nodes\n# %s\n\n%s",
				title, url, len(flat), time.Now().Format(time.RFC3339),
				bridge.FormatSnapshotText(flat))
			content = []byte(textContent)
		case "yaml":
			filename = fmt.Sprintf("snapshot-%s.yaml", timestamp)
			data := map[string]any{
				"url":       url,
				"title":     title,
				"timestamp": time.Now().Format(time.RFC3339),
				"nodes":     flat,
				"count":     len(flat),
			}
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
			filename = fmt.Sprintf("snapshot-%s.json", timestamp)
			data := map[string]any{
				"url":       url,
				"title":     title,
				"timestamp": time.Now().Format(time.RFC3339),
				"nodes":     flat,
				"count":     len(flat),
			}
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

		filePath := filepath.Join(snapshotDir, filename)
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
		}
		if err := os.WriteFile(filePath, content, 0600); err != nil {
			httpx.Error(w, 500, fmt.Errorf("write snapshot: %w", err))
			return
		}

		httpx.JSON(w, 200, map[string]any{
			"path":      filePath,
			"size":      len(content),
			"format":    format,
			"timestamp": timestamp,
		})
		return
	}

	if doDiff && prevNodes != nil {
		added, changed, removed := bridge.DiffSnapshot(prevNodes, flat)

		// Compact diff format: show all nodes with [+]/[~] markers
		if format == "compact" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(200)
			_, _ = fmt.Fprintf(w, "# %s | %s | %d nodes | +%d ~%d -%d",
				title, url, len(flat), len(added), len(changed), len(removed))
			if truncated {
				_, _ = fmt.Fprintf(w, " (truncated to ~%d tokens)", maxTokens)
			}
			_, _ = w.Write([]byte("\n"))
			content := bridge.FormatSnapshotCompactDiff(flat, added, changed, removed)
			if wrapContent {
				content = h.IDPIGuard.WrapContent(content, url)
			}
			_, _ = w.Write([]byte(content))
			return
		}

		httpx.JSON(w, 200, map[string]any{
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
		})
		return
	}

	switch format {
	case "compact":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "# %s | %s | %d nodes", title, url, len(flat))
		if truncated {
			_, _ = fmt.Fprintf(w, " (truncated to ~%d tokens)", maxTokens)
		}
		_, _ = w.Write([]byte("\n"))
		if scopedEmptyHint != "" {
			_, _ = fmt.Fprintf(w, "# hint: %s\n", scopedEmptyHint)
		}
		content := bridge.FormatSnapshotCompact(flat)
		if wrapContent {
			content = h.IDPIGuard.WrapContent(content, url)
		}
		_, _ = w.Write([]byte(content))
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "# %s\n# %s\n# %d nodes\n", title, url, len(flat))
		if scopedEmptyHint != "" {
			_, _ = fmt.Fprintf(w, "# hint: %s\n", scopedEmptyHint)
		}
		_, _ = w.Write([]byte("\n"))
		content := bridge.FormatSnapshotText(flat)
		if wrapContent {
			content = h.IDPIGuard.WrapContent(content, url)
		}
		_, _ = w.Write([]byte(content))
	case "yaml":
		data := map[string]any{
			"url":   url,
			"title": title,
			"nodes": flat,
			"count": len(flat),
		}
		if scopedEmptyHint != "" {
			data["hint"] = scopedEmptyHint
		}
		yamlContent, err := yaml.Marshal(data)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("marshal yaml: %w", err))
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write(yamlContent)
	default:
		resp := map[string]any{
			"url":   url,
			"title": title,
			"route": snapChromeRoute,
			"nodes": flat,
			"count": len(flat),
		}
		if truncated {
			resp["truncated"] = true
			resp["maxTokens"] = maxTokens
		}
		if scopedEmptyHint != "" {
			resp["hint"] = scopedEmptyHint
		}
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

// @Endpoint GET /tabs/{id}/snapshot
func (h *Handlers) HandleTabSnapshot(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleSnapshot)
}
