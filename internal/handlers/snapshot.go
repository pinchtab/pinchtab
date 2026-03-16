package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/idpi"
	"github.com/pinchtab/pinchtab/internal/web"
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

	// --- Lite engine fast path ---
	tabID := r.URL.Query().Get("tabId")
	if h.useLite(engine.CapSnapshot, "") {
		nodes, err := h.Router.Lite().Snapshot(r.Context(), tabID, filter)
		if err != nil {
			web.Error(w, 500, fmt.Errorf("lite snapshot: %w", err))
			return
		}
		// Convert to bridge.A11yNode for API compatibility.
		flat := make([]bridge.A11yNode, len(nodes))
		for i, n := range nodes {
			flat[i] = bridge.A11yNode{Ref: n.Ref, Role: n.Role, Name: n.Name, Depth: n.Depth, Value: n.Value}
		}
		w.Header().Set("X-Engine", "lite")
		web.JSON(w, 200, map[string]any{"nodes": flat})
		return
	}

	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	// filter and tabID already parsed above for lite path
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

	ctx, resolvedTabID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if reqNoAnim && !h.Config.NoAnimations {
		if err := bridge.DisableAnimationsOnce(tCtx); err != nil {
			web.Error(w, 500, fmt.Errorf("disable animations: %w", err))
			return
		}
	}

	nodes, err := bridge.FetchAXTree(tCtx)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("a11y tree: %w", err))
		return
	}
	treeResp := struct {
		Nodes []bridge.RawAXNode `json:"nodes"`
	}{Nodes: nodes}

	if selector != "" {
		// Unified selector: resolve to a backend node ID for subtree scoping.
		// Supports CSS (default), XPath, and text selectors.
		var scopeNodeID int64
		var scopeErr error

		switch {
		case strings.HasPrefix(selector, "xpath:"):
			scopeNodeID, scopeErr = bridge.ResolveXPathToNodeID(tCtx, selector[len("xpath:"):])
		case strings.HasPrefix(selector, "//") || strings.HasPrefix(selector, "(//"):
			scopeNodeID, scopeErr = bridge.ResolveXPathToNodeID(tCtx, selector)
		case strings.HasPrefix(selector, "text:"):
			scopeNodeID, scopeErr = bridge.ResolveTextToNodeID(tCtx, selector[len("text:"):])
		case strings.HasPrefix(selector, "css:"):
			scopeNodeID, scopeErr = bridge.ResolveCSSToNodeID(tCtx, selector[len("css:"):])
		default:
			// Bare selector — treat as CSS (backward compatible)
			scopeNodeID, scopeErr = bridge.ResolveCSSToNodeID(tCtx, selector)
		}

		if scopeErr != nil {
			web.Error(w, 400, fmt.Errorf("selector: %w", scopeErr))
			return
		}

		treeResp.Nodes = bridge.FilterSubtree(treeResp.Nodes, scopeNodeID)
	}

	flat, refs := bridge.BuildSnapshot(treeResp.Nodes, filter, maxDepth)

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

	h.Bridge.SetRefCache(resolvedTabID, &bridge.RefCache{Refs: refs, Nodes: flat})

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	// IDPI: scan accessibility-tree node names and values for injection patterns.
	// The scan runs after the snapshot is built so truncation has already reduced
	// the corpus. Headers are set before any write so they always reach the client.
	var idpiResult idpi.CheckResult
	if h.Config.IDPI.Enabled && h.Config.IDPI.ScanContent {
		var sb strings.Builder
		for _, n := range flat {
			// Join Name and Value within the same node with a space so multi-word
			// fields are scanned as a unit. Separate different nodes with \n so
			// that injection phrases split across node boundaries are not merged
			// into a false positive by the concatenation.
			if n.Name != "" || n.Value != "" {
				sb.WriteString(n.Name)
				if n.Name != "" && n.Value != "" {
					sb.WriteByte(' ')
				}
				sb.WriteString(n.Value)
				sb.WriteByte('\n')
			}
		}
		idpiResult = idpi.ScanContent(sb.String(), h.Config.IDPI)
		if idpiResult.Blocked {
			web.Error(w, http.StatusForbidden,
				fmt.Errorf("snapshot blocked by IDPI scanner: %s", idpiResult.Reason))
			return
		}
		if idpiResult.Threat {
			w.Header().Set("X-IDPI-Warning", idpiResult.Reason)
			if idpiResult.Pattern != "" {
				w.Header().Set("X-IDPI-Pattern", idpiResult.Pattern)
			}
		}
	}

	if output == "file" {
		snapshotDir := filepath.Join(h.Config.StateDir, "snapshots")
		if err := os.MkdirAll(snapshotDir, 0750); err != nil {
			web.Error(w, 500, fmt.Errorf("create snapshot dir: %w", err))
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
				web.Error(w, 500, fmt.Errorf("marshal yaml: %w", err))
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
				web.Error(w, 500, fmt.Errorf("marshal snapshot: %w", err))
				return
			}
		}

		filePath := filepath.Join(snapshotDir, filename)
		if outputPath != "" {
			safe, err := web.SafePath(h.Config.StateDir, outputPath)
			if err != nil {
				web.Error(w, 400, fmt.Errorf("invalid path: %w", err))
				return
			}
			absBase, _ := filepath.Abs(h.Config.StateDir)
			absPath, err := filepath.Abs(safe)
			if err != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
				web.Error(w, 400, fmt.Errorf("invalid output path"))
				return
			}
			filePath = absPath
			if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
				web.Error(w, 500, fmt.Errorf("create output dir: %w", err))
				return
			}
		}
		if err := os.WriteFile(filePath, content, 0600); err != nil {
			web.Error(w, 500, fmt.Errorf("write snapshot: %w", err))
			return
		}

		web.JSON(w, 200, map[string]any{
			"path":      filePath,
			"size":      len(content),
			"format":    format,
			"timestamp": timestamp,
		})
		return
	}

	if doDiff && prevNodes != nil {
		added, changed, removed := bridge.DiffSnapshot(prevNodes, flat)
		web.JSON(w, 200, map[string]any{
			"url":     url,
			"title":   title,
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
		_, _ = w.Write([]byte(bridge.FormatSnapshotCompact(flat)))
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "# %s\n# %s\n# %d nodes\n\n", title, url, len(flat))
		_, _ = w.Write([]byte(bridge.FormatSnapshotText(flat)))
	case "yaml":
		data := map[string]any{
			"url":   url,
			"title": title,
			"nodes": flat,
			"count": len(flat),
		}
		yamlContent, err := yaml.Marshal(data)
		if err != nil {
			web.Error(w, 500, fmt.Errorf("marshal yaml: %w", err))
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write(yamlContent)
	default:
		resp := map[string]any{
			"url":   url,
			"title": title,
			"nodes": flat,
			"count": len(flat),
		}
		if truncated {
			resp["truncated"] = true
			resp["maxTokens"] = maxTokens
		}
		if idpiResult.Threat {
			resp["idpiWarning"] = idpiResult.Reason
		}
		web.JSON(w, 200, resp)
	}
}

// HandleTabSnapshot returns snapshot for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/snapshot
func (h *Handlers) HandleTabSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.HandleSnapshot(w, req)
}
