package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
)

// ── GET /snapshot ──────────────────────────────────────────

func (b *Bridge) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	filter := r.URL.Query().Get("filter")
	doDiff := r.URL.Query().Get("diff") == "true"
	format := r.URL.Query().Get("format") // "text" for indented tree
	output := r.URL.Query().Get("output") // "file" to save to disk
	maxDepthStr := r.URL.Query().Get("depth")
	maxDepth := -1
	if maxDepthStr != "" {
		if d, err := strconv.Atoi(maxDepthStr); err == nil {
			maxDepth = d
		}
	}

	ctx, resolvedTabID, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var rawResult json.RawMessage
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Accessibility.getFullAXTree", nil, &rawResult)
		}),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("a11y tree: %w", err))
		return
	}

	var treeResp struct {
		Nodes []rawAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		jsonErr(w, 500, fmt.Errorf("parse a11y tree: %w", err))
		return
	}

	flat, refs := buildSnapshot(treeResp.Nodes, filter, maxDepth)

	// Get previous snapshot for diff before overwriting cache
	var prevNodes []A11yNode
	if doDiff {
		if prev := b.GetRefCache(resolvedTabID); prev != nil {
			prevNodes = prev.nodes
		}
	}

	// Cache ref→nodeID mapping and nodes for this tab
	b.SetRefCache(resolvedTabID, &refCache{refs: refs, nodes: flat})

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	// Handle file output
	if output == "file" {
		// Create snapshots directory if it doesn't exist
		snapshotDir := filepath.Join(stateDir, "snapshots")
		if err := os.MkdirAll(snapshotDir, 0755); err != nil {
			jsonErr(w, 500, fmt.Errorf("create snapshot dir: %w", err))
			return
		}

		// Generate filename with timestamp
		timestamp := time.Now().Format("20060102-150405")
		var filename string
		var content []byte

		if format == "text" {
			filename = fmt.Sprintf("snapshot-%s.txt", timestamp)
			textContent := fmt.Sprintf("# %s\n# %s\n# %d nodes\n# %s\n\n%s",
				title, url, len(flat), time.Now().Format(time.RFC3339),
				formatSnapshotText(flat))
			content = []byte(textContent)
		} else {
			filename = fmt.Sprintf("snapshot-%s.json", timestamp)
			data := map[string]any{
				"url":       url,
				"title":     title,
				"timestamp": time.Now().Format(time.RFC3339),
				"nodes":     flat,
				"count":     len(flat),
			}
			if doDiff && prevNodes != nil {
				added, changed, removed := diffSnapshot(prevNodes, flat)
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
				jsonErr(w, 500, fmt.Errorf("marshal snapshot: %w", err))
				return
			}
		}

		// Write to file
		filePath := filepath.Join(snapshotDir, filename)
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			jsonErr(w, 500, fmt.Errorf("write snapshot: %w", err))
			return
		}

		// Return path instead of data
		jsonResp(w, 200, map[string]any{
			"path":      filePath,
			"size":      len(content),
			"format":    format,
			"timestamp": timestamp,
		})
		return
	}

	if doDiff && prevNodes != nil {
		added, changed, removed := diffSnapshot(prevNodes, flat)
		jsonResp(w, 200, map[string]any{
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

	if format == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "# %s\n# %s\n# %d nodes\n\n", title, url, len(flat))
		_, _ = w.Write([]byte(formatSnapshotText(flat)))
		return
	}

	jsonResp(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"nodes": flat,
		"count": len(flat),
	})
}
